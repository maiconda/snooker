import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import {
  SnookerMatch,
  type CueTelemetry,
  type MatchHudState,
  type MatchSnapshot,
  type Scoreboard,
  type ShotStartedEvent
} from "../game/SnookerMatch";
import { navigate } from "../lib/router";
import { getRoom } from "../lobby/lobbyApi";
import type {
  CueStatePayload,
  MatchFinishedPayload,
  RematchRequestedPayload,
  Room,
  RoomSpectatorsSnapshot,
  WSEvent,
  XPAward
} from "../lobby/types";
import { getPublicProfile } from "../profile/profileApi";
import type { Profile } from "../profile/types";

type ChatMessage = {
  messageId: string;
  senderId: string;
  senderName: string;
  text: string;
  timestamp: number;
};

type RemoteCueState = CueTelemetry & {
  senderId: string;
  client_seq: number;
};

type RoomEventPayload = {
  room?: Room;
  user_id?: string;
  message_id?: string;
  text?: string;
  created_at?: string;
  reason?: string;
  disconnects_at?: string;
};

type MatchSummary = {
  winnerUserId?: string;
  scores: Scoreboard;
  xpAwards: XPAward[];
};

const CUE_SEND_INTERVAL_MS = 33;
const INITIAL_SCORES: Scoreboard = { creator: 0, opponent: 0 };

type SnapshotVersion = {
  shotSeq: number;
  updatedAtMs: number;
};

function normalizeAngle(value: number): number {
  const fullTurn = Math.PI * 2;
  let normalized = value % fullTurn;
  if (normalized <= -Math.PI) normalized += fullTurn;
  if (normalized > Math.PI) normalized -= fullTurn;
  return normalized;
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function isUsableCuePayload(cue: CueStatePayload): boolean {
  return (
    isFiniteNumber(cue.x) &&
    isFiniteNumber(cue.y) &&
    isFiniteNumber(cue.angle) &&
    isFiniteNumber(cue.power) &&
    cue.power >= 0 &&
    cue.power <= 100 &&
    typeof cue.shot_seq === "number"
  );
}

function isNewerSnapshot(snapshot: MatchSnapshot, current: SnapshotVersion): boolean {
  if (!isFiniteNumber(snapshot.updated_at_ms) || typeof snapshot.shot_seq !== "number") {
    return false;
  }
  if (snapshot.shot_seq < current.shotSeq) {
    return false;
  }
  if (snapshot.shot_seq === current.shotSeq && snapshot.updated_at_ms <= current.updatedAtMs) {
    return false;
  }
  return true;
}

export function GamePage({ roomId }: { roomId: string }) {
  const { session } = useAuth();
  const [room, setRoom] = useState<Room | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [messageText, setMessageText] = useState("");
  const [chatOpen, setChatOpen] = useState(false);
  const [unreadMessages, setUnreadMessages] = useState(0);
  const [remoteCue, setRemoteCue] = useState<RemoteCueState | null>(null);
  const [incomingShot, setIncomingShot] = useState<ShotStartedEvent | null>(null);
  const [incomingSnapshot, setIncomingSnapshot] = useState<MatchSnapshot | null>(null);
  const [scores, setScores] = useState<Scoreboard>(INITIAL_SCORES);
  const [turnUserId, setTurnUserId] = useState("");
  const [hud, setHud] = useState<MatchHudState>({
    angle: 0,
    power: 50,
    status: "aiming",
    canShoot: false
  });
  const [spectators, setSpectators] = useState(0);
  const [profilesById, setProfilesById] = useState<Record<string, Profile>>({});
  const [matchSummary, setMatchSummary] = useState<MatchSummary | null>(null);
  const [requestedRematches, setRequestedRematches] = useState<string[]>([]);
  const [resetKey, setResetKey] = useState(roomId);
  const [stateSyncKey, setStateSyncKey] = useState("");
  const [isNewMatch, setIsNewMatch] = useState(() => {
    try {
      const isNew = Boolean(window.history.state?.isNewMatch);
      if (isNew) {
        window.history.replaceState(null, "", window.location.pathname + window.location.search);
      }
      return isNew;
    } catch (e) {
      console.warn("History state error:", e);
      return false;
    }
  });

  const wsRef = useRef<WebSocket | null>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const profilesCache = useRef<Record<string, Profile>>({});
  const chatMessageIdsRef = useRef<Set<string>>(new Set());
  const lastCueSeqBySender = useRef<Record<string, number>>({});
  const clientSeqRef = useRef(0);
  const lastCueSentAtRef = useRef(0);
  const chatOpenRef = useRef(false);
  const lastSnapshotVersionRef = useRef<SnapshotVersion>({ shotSeq: -1, updatedAtMs: 0 });
  const roomRef = useRef<Room | null>(null);
  const scoresRef = useRef<Scoreboard>(INITIAL_SCORES);
  roomRef.current = room;
  scoresRef.current = scores;

  const userId = session?.userId ?? "";
  const isCreator = Boolean(room && userId === room.creator_id);
  const isOpponent = Boolean(room?.opponent_id && userId === room.opponent_id);
  const isParticipant = isCreator || isOpponent;
  const creatorName = room ? profilesById[room.creator_id]?.nickname ?? "Dono" : "Dono";
  const opponentName = room?.opponent_id ? profilesById[room.opponent_id]?.nickname ?? "Oponente" : "Oponente";
  const activeName = turnUserId ? profilesById[turnUserId]?.nickname ?? "Jogador" : creatorName;
  const winnerName = matchSummary?.winnerUserId
    ? profilesById[matchSummary.winnerUserId]?.nickname ?? "Jogador"
    : "";
  const creatorDisconnected = Boolean(room?.creator_disconnected_at);
  const opponentDisconnected = Boolean(room?.opponent_disconnected_at);
  const hasReconnectingPlayer = creatorDisconnected || opponentDisconnected;
  const currentUserRequestedRematch = requestedRematches.includes(userId);

  const xpByUserId = useMemo(() => {
    const result: Record<string, XPAward> = {};
    for (const award of matchSummary?.xpAwards ?? []) {
      result[award.user_id] = award;
    }
    return result;
  }, [matchSummary?.xpAwards]);

  const rememberProfile = (profileUserId: string, profile: Profile) => {
    profilesCache.current[profileUserId] = profile;
    setProfilesById((prev) => ({ ...prev, [profileUserId]: profile }));
  };

  const ensureProfile = async (token: string, profileUserId: string, force = false) => {
    if (!profilesCache.current[profileUserId] || force) {
      profilesCache.current[profileUserId] = await getPublicProfile(token, profileUserId);
    }
    rememberProfile(profileUserId, profilesCache.current[profileUserId]);
    return profilesCache.current[profileUserId];
  };

  const sendWS = useCallback((type: string, payload: unknown) => {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;
    wsRef.current.send(JSON.stringify({ type, payload }));
  }, []);

  const acceptSnapshot = useCallback((snapshot: MatchSnapshot) => {
    if (!isNewerSnapshot(snapshot, lastSnapshotVersionRef.current)) {
      return false;
    }
    lastSnapshotVersionRef.current = {
      shotSeq: snapshot.shot_seq,
      updatedAtMs: snapshot.updated_at_ms
    };
    return true;
  }, []);

  useEffect(() => {
    chatOpenRef.current = chatOpen;
    if (chatOpen) {
      setUnreadMessages(0);
    }
  }, [chatOpen]);

  useEffect(() => {
    lastSnapshotVersionRef.current = { shotSeq: -1, updatedAtMs: 0 };
  }, [roomId]);

  useEffect(() => {
    let active = true;
    const token = session?.accessToken;
    if (!token) return;
    const activeToken = token;

    async function loadRoom() {
      try {
        setLoading(true);
        const roomData = await getRoom(activeToken, roomId);
        if (!active) return;

        setRoom(roomData);
        setTurnUserId(roomData.creator_id);
        if (roomData.status === "waiting" || roomData.status === "expired") {
          navigate(`/sala/${roomData.id}`);
          return;
        }
        if (roomData.status === "finished") {
          setMatchSummary({ scores: INITIAL_SCORES, xpAwards: [] });
        }

        void ensureProfile(activeToken, roomData.creator_id).catch(() => undefined);
        if (roomData.opponent_id) {
          void ensureProfile(activeToken, roomData.opponent_id).catch(() => undefined);
        }
      } catch (err) {
        if (active) {
          setError(err instanceof Error ? err.message : "Falha ao carregar a partida.");
        }
      } finally {
        if (active) setLoading(false);
      }
    }

    loadRoom();
    return () => {
      active = false;
    };
  }, [roomId, session?.accessToken]);

  useEffect(() => {
    const token = session?.accessToken;
    const activeRoomId = room?.id;
    if (!token || !activeRoomId) return;

    let active = true;
    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/rooms/${activeRoomId}/ws?token=${token}`);
      wsRef.current = ws;

      ws.onopen = () => {
        if (!isNewMatch && roomRef.current?.status === "playing") {
          ws?.send(JSON.stringify({ type: "request_game_state", payload: {} }));
        }
      };

      ws.onmessage = async (event) => {
        if (!active) return;
        try {
          const wsMsg = JSON.parse(event.data) as WSEvent;
          const senderId = wsMsg.sender_id ?? "";
          const rawPayload = wsMsg.payload as unknown;
          const payload = (rawPayload && typeof rawPayload === "object" ? rawPayload : {}) as RoomEventPayload;
          const roomFromPayload = rawPayload && typeof rawPayload === "object" && "id" in rawPayload
            ? (rawPayload as Room)
            : payload.room;

          if (senderId) {
            void ensureProfile(token, senderId).catch(() => undefined);
          }

          switch (wsMsg.type) {
            case "room_state":
              if (roomFromPayload) {
                setRoom(roomFromPayload);
                if (roomFromPayload.status === "waiting" || roomFromPayload.status === "expired") {
                  navigate(`/sala/${roomFromPayload.id}`);
                }
              }
              break;

            case "owner_reconnected":
            case "player_reconnected":
            case "player_joined":
              if (roomFromPayload) {
                setRoom(roomFromPayload);
                if (roomFromPayload.status === "waiting" || roomFromPayload.status === "expired") {
                  navigate(`/sala/${roomFromPayload.id}`);
                }
              }
              setStateSyncKey(`${wsMsg.type}:${Date.now()}`);
              break;

            case "player_left":
              if (roomFromPayload) {
                setRoom(roomFromPayload);
                if (roomFromPayload.status === "waiting" || roomFromPayload.status === "expired") {
                  navigate(`/sala/${roomFromPayload.id}`);
                }
              }
              break;

            case "owner_disconnected":
              setRoom((prev) => prev
                ? { ...prev, creator_disconnected_at: payload.disconnects_at ?? new Date().toISOString() }
                : prev);
              break;

            case "player_disconnected":
              setRoom((prev) => {
                if (!prev || senderId === prev.creator_id) return prev;
                return { ...prev, opponent_disconnected_at: payload.disconnects_at ?? new Date().toISOString() };
              });
              break;

            case "room_spectators_snapshot":
              if (rawPayload && typeof rawPayload === "object" && "spectators" in rawPayload) {
                const snapshot = rawPayload as RoomSpectatorsSnapshot;
                setSpectators(snapshot.count);
                setStateSyncKey(`spectators:${Date.now()}`);
              }
              break;

            case "chat_message": {
              const chatText = payload.text;
              if (!chatText) break;
              const createdAt = payload.created_at ?? "";
              const messageId = payload.message_id ?? `${senderId}:${createdAt}:${chatText}`;
              if (chatMessageIdsRef.current.has(messageId)) break;
              chatMessageIdsRef.current.add(messageId);

              let senderName = "Jogador";
              if (senderId) {
                try {
                  const profile = await ensureProfile(token, senderId);
                  senderName = profile.nickname;
                } catch {
                  senderName = "Jogador";
                }
              }

              const parsedCreatedAt = createdAt ? Date.parse(createdAt) : NaN;
              setMessages((prev) => [
                ...prev,
                {
                  messageId,
                  senderId,
                  senderName,
                  text: chatText,
                  timestamp: Number.isNaN(parsedCreatedAt) ? Date.now() : parsedCreatedAt
                }
              ]);
              if (!chatOpenRef.current) {
                setUnreadMessages((current) => current + 1);
              }
              break;
            }

            case "cue_state": {
              if (!senderId || senderId === userId) break;
              const cue = payload as unknown as CueStatePayload;
              const previousSeq = lastCueSeqBySender.current[senderId] ?? -1;
              const nextSeq = typeof cue.client_seq === "number" ? cue.client_seq : 0;
              if (nextSeq <= previousSeq || !isUsableCuePayload(cue)) break;
              lastCueSeqBySender.current[senderId] = nextSeq;
              setRemoteCue({
                senderId,
                client_seq: nextSeq,
                shot_seq: cue.shot_seq,
                turn_user_id: cue.turn_user_id ?? senderId,
                x: cue.x,
                y: cue.y,
                angle: normalizeAngle(cue.angle),
                power: cue.power,
                is_aiming: cue.is_aiming
              });
              break;
            }

            case "shot_started":
              if (senderId !== userId) {
                setIncomingShot(rawPayload as ShotStartedEvent);
              }
              break;

            case "request_game_state":
              if (senderId !== userId) {
                setStateSyncKey(`request_game_state:${Date.now()}`);
              }
              break;

            case "game_state_sync": {
              const snapshot = rawPayload as MatchSnapshot;
              if (!snapshot?.scores || !acceptSnapshot(snapshot)) {
                break;
              }
              if (senderId !== userId && snapshot?.updated_at_ms) {
                setIncomingSnapshot(snapshot);
                setIsNewMatch(false);
              }
              setScores(snapshot.scores);
              if (snapshot?.turn_user_id) {
                setTurnUserId(snapshot.turn_user_id);
              }
              if (snapshot?.status === "finished") {
                setMatchSummary((prev) => ({
                  winnerUserId: snapshot.winner_user_id ?? prev?.winnerUserId,
                  scores: snapshot.scores,
                  xpAwards: prev?.xpAwards ?? []
                }));
              }
              break;
            }

            case "match_finished": {
              const matchPayload = payload as MatchFinishedPayload;
              const nextRoom = matchPayload.room ?? roomRef.current;
              if (nextRoom) {
                setRoom(nextRoom);
              }
              setMatchSummary((prev) => ({
                winnerUserId: matchPayload.winner_user_id ?? prev?.winnerUserId,
                scores: prev?.scores ?? scoresRef.current,
                xpAwards: matchPayload.xp_awards ?? prev?.xpAwards ?? []
              }));
              for (const award of matchPayload.xp_awards ?? []) {
                void ensureProfile(token, award.user_id, true).catch(() => undefined);
              }
              break;
            }

            case "match_start":
              if (roomFromPayload) {
                setRoom(roomFromPayload);
                setMatchSummary(null);
                setRequestedRematches([]);
                setScores(INITIAL_SCORES);
                setTurnUserId(roomFromPayload.creator_id);
                setIncomingSnapshot(null);
                setIncomingShot(null);
                setRemoteCue(null);
                setIsNewMatch(true);
                lastSnapshotVersionRef.current = { shotSeq: -1, updatedAtMs: 0 };
                setResetKey(`${roomFromPayload.id}:${Date.now()}`);
              }
              break;

            case "room_reset":
              if (roomFromPayload) {
                setRoom(roomFromPayload);
                setMatchSummary(null);
                setRequestedRematches([]);
                setScores(INITIAL_SCORES);
                setTurnUserId(roomFromPayload.creator_id);
                setIncomingSnapshot(null);
                setIncomingShot(null);
                setRemoteCue(null);
                lastSnapshotVersionRef.current = { shotSeq: -1, updatedAtMs: 0 };
                setResetKey(`${roomFromPayload.id}:${Date.now()}`);
                navigate(`/sala/${roomFromPayload.id}`);
              }
              break;

            case "rematch_requested": {
              const rematchPayload = payload as RematchRequestedPayload;
              if (rematchPayload.requested_user_ids) {
                setRequestedRematches(rematchPayload.requested_user_ids);
              } else if (rematchPayload.user_id) {
                setRequestedRematches((prev) => Array.from(new Set([...prev, rematchPayload.user_id])));
              }
              if (rematchPayload.user_id) {
                void ensureProfile(token, rematchPayload.user_id).catch(() => undefined);
              }
              break;
            }

            case "room_closed":
              setError(payload.reason === "owner_closed" ? "A sala foi encerrada pelo dono." : "A sala foi encerrada.");
              navigate("/");
              break;

            default:
              break;
          }
        } catch (err) {
          console.error("Erro ao processar evento da partida:", err);
        }
      };

      ws.onclose = () => {
        if (!active) return;
        console.warn("WebSocket da partida desconectado. Reconectando em 2s...");
        reconnectTimer = setTimeout(connect, 2000);
      };

      ws.onerror = (err) => {
        console.error("Erro no WebSocket da partida:", err);
      };
    };

    connect();

    return () => {
      active = false;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, [room?.id, session?.accessToken, userId]);

  useEffect(() => {
    if (chatOpen) {
      chatEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages, chatOpen]);

  const sendCueState = useCallback((cue: CueTelemetry) => {
    if (!room || !wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) return;

    const now = Date.now();
    if (now - lastCueSentAtRef.current < CUE_SEND_INTERVAL_MS) return;
    lastCueSentAtRef.current = now;
    clientSeqRef.current += 1;

    const payload: CueStatePayload = {
      ...cue,
      angle: normalizeAngle(cue.angle),
      match_id: room.id,
      client_seq: clientSeqRef.current
    };

    sendWS("cue_state", payload);
  }, [room?.id, sendWS]);

  const handleSnapshot = useCallback((snapshot: MatchSnapshot) => {
    if (!acceptSnapshot(snapshot)) {
      return;
    }
    setScores(snapshot.scores);
    setTurnUserId(snapshot.turn_user_id);
    if (snapshot.status === "finished") {
      setMatchSummary((prev) => ({
        winnerUserId: snapshot.winner_user_id ?? prev?.winnerUserId,
        scores: snapshot.scores,
        xpAwards: prev?.xpAwards ?? []
      }));
    }
    sendWS("game_state_sync", snapshot);
    setIsNewMatch(false);
  }, [acceptSnapshot, sendWS, setIsNewMatch]);

  const handleMatchFinished = useCallback((summary: { winnerUserId: string; scores: Scoreboard }) => {
    setMatchSummary((prev) => ({
      winnerUserId: summary.winnerUserId,
      scores: summary.scores,
      xpAwards: prev?.xpAwards ?? []
    }));
    sendWS("match_end", {
      reason: "normal",
      winner_user_id: summary.winnerUserId,
      scores: summary.scores
    });
  }, [sendWS]);

  const handleLocalShotStarted = useCallback((shot: ShotStartedEvent) => {
    sendWS("shot_started", shot);
  }, [sendWS]);

  const handleRequestGameState = useCallback(() => {
    sendWS("request_game_state", {});
  }, [sendWS]);

  const handleSendChat = (event: FormEvent) => {
    event.preventDefault();
    if (!isParticipant || !messageText.trim()) return;
    sendWS("chat_message", { text: messageText });
    setMessageText("");
  };

  const handleRequestRematch = () => {
    sendWS("rematch_request", {});
    setRequestedRematches((prev) => Array.from(new Set([...prev, userId])));
  };

  const handleCloseRoom = () => {
    sendWS("room_close_request", {});
  };

  if (loading) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-neutral-950 text-white">
        <p className="text-sm text-neutral-400">Carregando partida...</p>
      </main>
    );
  }

  if (error || !room) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center bg-neutral-950 px-6 text-white">
        <h1 className="text-xl font-semibold text-red-500">Partida indisponivel</h1>
        <p className="mt-2 text-sm text-neutral-400">{error ?? "Sala invalida."}</p>
        <Button onClick={() => navigate("/")} className="mt-8 max-w-xs">
          Voltar
        </Button>
      </main>
    );
  }

  return (
    <main className="relative min-h-screen overflow-hidden overflow-x-hidden bg-neutral-950 text-white">
      <section className="absolute inset-0">
        <SnookerMatch
          creatorId={room.creator_id}
          opponentId={room.opponent_id}
          currentUserId={userId}
          remoteCue={remoteCue}
          incomingShot={incomingShot}
          incomingSnapshot={incomingSnapshot}
          disabled={hasReconnectingPlayer || room.status === "finished"}
          resetKey={resetKey}
          onCueState={sendCueState}
          onLocalShotStarted={handleLocalShotStarted}
          onSnapshot={handleSnapshot}
          onMatchFinished={handleMatchFinished}
          onHudChange={setHud}
          requestStateSyncKey={stateSyncKey}
          isNewMatch={isNewMatch}
          onRequestGameState={handleRequestGameState}
        />
      </section>

      <header className="pointer-events-none absolute inset-x-0 top-0 z-30 p-3 md:p-4">
        <div className="pointer-events-auto mx-auto grid max-w-7xl gap-3 md:grid-cols-[1fr_auto]">
          <div className="grid gap-3 border border-white/10 bg-neutral-950/80 p-3 shadow-xl backdrop-blur md:grid-cols-[1fr_auto] md:items-center">
            <div>
              <p className="text-xs uppercase tracking-[0.18em] text-emerald-300">Partida em andamento</p>
              <h1 className="mt-1 text-lg font-semibold tracking-normal md:text-xl">
                {creatorName} vs {opponentName}
              </h1>
            </div>
            <div className="grid grid-cols-2 gap-2 text-xs md:min-w-[360px]">
              <ScoreTile
                name={creatorName}
                label="Amarelos"
                score={scores.creator}
                active={turnUserId === room.creator_id}
                reconnecting={creatorDisconnected}
                tone="creator"
                xpAward={xpByUserId[room.creator_id]}
              />
              <ScoreTile
                name={opponentName}
                label="Azuis"
                score={scores.opponent}
                active={turnUserId === room.opponent_id}
                reconnecting={opponentDisconnected}
                tone="opponent"
                xpAward={room.opponent_id ? xpByUserId[room.opponent_id] : undefined}
              />
            </div>
          </div>

          <div className="flex flex-wrap items-stretch gap-2 md:justify-end">
            <StatusPill label="Turno" value={activeName} />
            <StatusPill label="Status" value={statusText(hud.status, room.status)} />
            <StatusPill label="Forca" value={`${Math.round(hud.power)}%`} />
            <StatusPill label="Espectadores" value={String(spectators)} />
            <button
              type="button"
              onClick={() => setChatOpen((open) => !open)}
              className="border border-white/15 bg-neutral-950/80 px-3 py-2 text-sm font-semibold text-white backdrop-blur transition hover:border-white/40"
            >
              Chat {unreadMessages > 0 && !chatOpen ? `(${unreadMessages})` : messages.length > 0 ? `(${messages.length})` : ""}
            </button>
          </div>
        </div>
      </header>

      {hasReconnectingPlayer && (
        <div className="pointer-events-none absolute left-1/2 top-32 z-30 w-[min(92vw,720px)] -translate-x-1/2 border border-amber-400/30 bg-amber-400/15 px-4 py-3 text-sm text-amber-50 shadow-xl backdrop-blur">
          {creatorDisconnected && opponentDisconnected
            ? "Os dois jogadores estao reconectando. A partida fica pausada para manter a simulacao consistente."
            : `${creatorDisconnected ? creatorName : opponentName} esta reconectando. A sala permanece viva enquanto aguardamos o retorno.`}
        </div>
      )}

      <aside
        className={`absolute bottom-4 right-3 top-32 z-40 flex w-[min(92vw,380px)] max-w-[calc(100vw-1.5rem)] flex-col overflow-hidden border border-white/10 bg-neutral-950/92 shadow-2xl backdrop-blur transition duration-200 md:right-4 md:max-w-[calc(100vw-2rem)] ${
          chatOpen ? "translate-x-0 opacity-100" : "pointer-events-none translate-x-[calc(100%+2rem)] opacity-0"
        }`}
      >
        <header className="flex items-center justify-between border-b border-white/10 px-4 py-3">
          <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-neutral-300">Chat</h2>
          <button
            type="button"
            onClick={() => setChatOpen(false)}
            className="h-8 w-8 border border-white/10 text-sm text-neutral-300 transition hover:border-white/40 hover:text-white"
            aria-label="Fechar chat"
          >
            x
          </button>
        </header>

        <div className="flex-1 space-y-3 overflow-y-auto p-4">
          {messages.length === 0 ? (
            <div className="flex h-full items-center justify-center text-center text-xs text-neutral-600">
              Nenhuma mensagem nesta partida.
            </div>
          ) : (
            messages.map((message) => (
              <div key={message.messageId} className={`flex w-full min-w-0 flex-col ${message.senderId === userId ? "items-end" : "items-start"}`}>
                <span className="mb-1 max-w-[86%] truncate px-1 text-[10px] text-neutral-500">{message.senderName}</span>
                <div className={`max-w-[86%] overflow-hidden break-words px-3 py-2 text-sm [overflow-wrap:anywhere] ${
                  message.senderId === userId
                    ? "bg-emerald-600 text-white"
                    : "bg-white/10 text-neutral-200"
                }`}>
                  {message.text}
                </div>
              </div>
            ))
          )}
          <div ref={chatEndRef} />
        </div>

        <form onSubmit={handleSendChat} className="flex gap-2 border-t border-white/10 p-3">
          <input
            type="text"
            value={messageText}
            onChange={(event) => setMessageText(event.target.value)}
            disabled={!isParticipant}
            placeholder={isParticipant ? "Mensagem..." : "Espectadores apenas leem"}
            className="min-w-0 flex-1 border border-white/10 bg-neutral-950 px-3 py-2 text-sm text-white placeholder-neutral-600 outline-none focus:border-emerald-500 disabled:opacity-50"
          />
          <button
            type="submit"
            disabled={!isParticipant || !messageText.trim()}
            className="border border-emerald-500 bg-emerald-600 px-3 text-sm font-semibold text-white transition hover:bg-emerald-500 disabled:cursor-not-allowed disabled:border-neutral-800 disabled:bg-neutral-800 disabled:text-neutral-600"
          >
            Enviar
          </button>
        </form>
      </aside>

      {matchSummary && (
        <MatchFinishedOverlay
          isCreator={isCreator}
          isParticipant={isParticipant}
          currentUserRequestedRematch={currentUserRequestedRematch}
          winnerName={winnerName}
          creatorName={creatorName}
          opponentName={opponentName}
          scores={matchSummary.scores}
          xpAwards={matchSummary.xpAwards}
          profilesById={profilesById}
          onRequestRematch={handleRequestRematch}
          onCloseRoom={handleCloseRoom}
          onLeave={() => navigate("/")}
          onLobby={() => navigate(`/sala/${room.id}`)}
        />
      )}
    </main>
  );
}

function ScoreTile({
  name,
  label,
  score,
  active,
  reconnecting,
  tone,
  xpAward
}: {
  name: string;
  label: string;
  score: number;
  active: boolean;
  reconnecting: boolean;
  tone: "creator" | "opponent";
  xpAward?: XPAward;
}) {
  const color = tone === "creator" ? "bg-amber-300" : "bg-sky-400";

  return (
    <div className={`border p-2 ${active ? "border-emerald-300/60 bg-emerald-400/10" : "border-white/10 bg-white/5"}`}>
      <div className="flex items-center justify-between gap-2">
        <span className={`h-2.5 w-2.5 ${color}`} />
        <span className="truncate text-[10px] uppercase tracking-[0.14em] text-neutral-500">{label}</span>
      </div>
      <p className="mt-1 truncate text-sm font-semibold text-white">{name}</p>
      <div className="mt-1 flex items-end justify-between gap-2">
        <strong className="text-2xl leading-none text-white">{score}</strong>
        <span className={`text-[10px] font-semibold ${reconnecting ? "text-amber-300" : active ? "text-emerald-300" : "text-neutral-500"}`}>
          {reconnecting ? "Reconectando" : active ? "Turno" : xpAward ? `+${xpAward.xp_delta} XP` : "Online"}
        </span>
      </div>
    </div>
  );
}

function StatusPill({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-24 border border-white/10 bg-neutral-950/80 px-3 py-2 backdrop-blur">
      <p className="text-[10px] uppercase tracking-[0.14em] text-neutral-500">{label}</p>
      <p className="truncate text-sm font-semibold text-white">{value}</p>
    </div>
  );
}

function MatchFinishedOverlay({
  isCreator,
  isParticipant,
  currentUserRequestedRematch,
  winnerName,
  creatorName,
  opponentName,
  scores,
  xpAwards,
  profilesById,
  onRequestRematch,
  onCloseRoom,
  onLeave,
  onLobby
}: {
  isCreator: boolean;
  isParticipant: boolean;
  currentUserRequestedRematch: boolean;
  winnerName: string;
  creatorName: string;
  opponentName: string;
  scores: Scoreboard;
  xpAwards: XPAward[];
  profilesById: Record<string, Profile>;
  onRequestRematch: () => void;
  onCloseRoom: () => void;
  onLeave: () => void;
  onLobby: () => void;
}) {
  return (
    <div className="absolute inset-0 z-50 flex items-center justify-center bg-neutral-950/72 p-4 backdrop-blur-sm">
      <section className="w-full max-w-xl border border-white/10 bg-neutral-950 p-5 text-white shadow-2xl">
        <p className="text-xs uppercase tracking-[0.18em] text-emerald-300">Partida finalizada</p>
        <h2 className="mt-2 text-2xl font-semibold tracking-normal">
          {winnerName ? `${winnerName} venceu` : "Resultado registrado"}
        </h2>

        <div className="mt-5 grid gap-3 sm:grid-cols-2">
          <ResultScore name={creatorName} label="Amarelos" score={scores.creator} />
          <ResultScore name={opponentName} label="Azuis" score={scores.opponent} />
        </div>

        <div className="mt-5 border border-white/10 bg-white/5 p-3">
          <p className="text-xs uppercase tracking-[0.14em] text-neutral-500">XP da partida</p>
          <div className="mt-3 grid gap-2">
            {xpAwards.length === 0 ? (
              <p className="text-sm text-neutral-400">Aguardando confirmacao do servidor.</p>
            ) : (
              xpAwards.map((award) => (
                <div key={award.user_id} className="flex items-center justify-between text-sm">
                  <span className="truncate text-neutral-300">{profilesById[award.user_id]?.nickname ?? "Jogador"}</span>
                  <strong className="text-emerald-300">+{award.xp_delta} XP</strong>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="mt-6 grid gap-3 sm:grid-cols-2">
          {isParticipant && (
            <Button onClick={onRequestRematch} disabled={currentUserRequestedRematch}>
              {currentUserRequestedRematch ? "Revanche solicitada" : "Pedir revanche"}
            </Button>
          )}
          {isCreator ? (
            <Button variant="outline" onClick={onCloseRoom}>
              Encerrar sala
            </Button>
          ) : (
            <Button variant="outline" onClick={onLeave}>
              Sair da sala
            </Button>
          )}
          <Button variant="outline" onClick={onLobby} className="sm:col-span-2">
            Voltar ao lobby
          </Button>
        </div>
      </section>
    </div>
  );
}

function ResultScore({ name, label, score }: { name: string; label: string; score: number }) {
  return (
    <div className="border border-white/10 bg-white/5 p-3">
      <p className="truncate text-sm font-semibold text-white">{name}</p>
      <p className="text-[10px] uppercase tracking-[0.14em] text-neutral-500">{label}</p>
      <strong className="mt-3 block text-3xl leading-none text-white">{score}</strong>
    </div>
  );
}

function statusText(gameStatus: MatchHudState["status"], roomStatus: Room["status"]) {
  if (roomStatus === "finished") return "Finalizada";
  switch (gameStatus) {
    case "aiming":
      return "Mirando";
    case "striking":
      return "Tacada";
    case "moving":
      return "Movimento";
    default:
      return "Finalizada";
  }
}
