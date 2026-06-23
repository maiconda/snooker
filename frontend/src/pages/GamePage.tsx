import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import {
  SnookerMatch,
  type CueTelemetry,
  type MatchHudState,
  type MatchSnapshot,
  type Scoreboard,
  type ShotResultSubmittedEvent,
  type ShotStartedEvent
} from "../game/SnookerMatch";
import { navigate } from "../lib/router";
import { getRoom } from "../lobby/lobbyApi";
import { getRoomClientId } from "../lobby/roomClient";
import type {
  CueStatePayload,
  MatchFinishedPayload,
  PresenceUser,
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
  server_received_at_ms: number;
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

type TurnTimeoutPayload = {
  turn_seq: number;
  timed_out_user_id: string;
  next_turn_user_id: string;
  next_turn_seq: number;
  turn_deadline_at_ms: number;
  turn_started_at_ms?: number;
};

const CUE_SEND_INTERVAL_MS = 33;
const INITIAL_SCORES: Scoreboard = { creator: 0, opponent: 0 };

type SnapshotVersion = {
  shotSeq: number;
  turnSeq: number;
  updatedAtMs: number;
  status?: MatchSnapshot["status"];
};

type CuePacketVersion = {
  shotSeq: number;
  clientSeq: number;
  serverReceivedAtMs: number;
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
    isFiniteNumber(cue.shot_seq) &&
    cue.shot_seq >= 0 &&
    isFiniteNumber(cue.client_seq) &&
    cue.client_seq >= 0
  );
}

function cuePacketVersion(cue: CueStatePayload): CuePacketVersion {
  return {
    shotSeq: cue.shot_seq,
    clientSeq: cue.client_seq,
    serverReceivedAtMs: isFiniteNumber(cue.server_received_at_ms) ? cue.server_received_at_ms : 0
  };
}

function isNewerCuePacket(next: CuePacketVersion, current?: CuePacketVersion): boolean {
  if (!current) return true;

  if (next.serverReceivedAtMs > 0 && current.serverReceivedAtMs > 0) {
    if (next.serverReceivedAtMs !== current.serverReceivedAtMs) {
      return next.serverReceivedAtMs > current.serverReceivedAtMs;
    }
  }

  if (next.shotSeq !== current.shotSeq) {
    return next.shotSeq > current.shotSeq;
  }

  if (next.clientSeq !== current.clientSeq) {
    return next.clientSeq > current.clientSeq || next.serverReceivedAtMs > current.serverReceivedAtMs;
  }

  return next.serverReceivedAtMs > current.serverReceivedAtMs;
}

function isNewerSnapshot(snapshot: MatchSnapshot, current: SnapshotVersion): boolean {
  if (
    !isFiniteNumber(snapshot.updated_at_ms) ||
    typeof snapshot.shot_seq !== "number" ||
    typeof snapshot.turn_seq !== "number"
  ) {
    return false;
  }

  if (snapshot.shot_seq !== current.shotSeq) {
    return snapshot.shot_seq > current.shotSeq;
  }
  if (snapshot.turn_seq !== current.turnSeq) {
    return snapshot.turn_seq > current.turnSeq;
  }
  if (snapshot.updated_at_ms !== current.updatedAtMs) {
    return snapshot.updated_at_ms > current.updatedAtMs;
  }

  return snapshotStatusRank(snapshot.status) > snapshotStatusRank(current.status);
}

function snapshotStatusRank(status?: MatchSnapshot["status"]): number {
  switch (status) {
    case "aiming":
      return 1;
    case "striking":
      return 2;
    case "moving":
      return 3;
    case "finished":
      return 4;
    default:
      return 0;
  }
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
  const [turnDeadlineAtMs, setTurnDeadlineAtMs] = useState(0);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [hud, setHud] = useState<MatchHudState>({
    angle: 0,
    power: 50,
    status: "aiming",
    canShoot: false
  });
  const [spectators, setSpectators] = useState(0);
  const [spectatorList, setSpectatorList] = useState<PresenceUser[]>([]);
  const [spectatorsOpen, setSpectatorsOpen] = useState(false);
  const [profilesById, setProfilesById] = useState<Record<string, Profile>>({});
  const [matchSummary, setMatchSummary] = useState<MatchSummary | null>(null);
  const [requestedRematches, setRequestedRematches] = useState<string[]>([]);
  const [resetKey, setResetKey] = useState(roomId);

  const wsRef = useRef<WebSocket | null>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const profilesCache = useRef<Record<string, Profile>>({});
  const chatMessageIdsRef = useRef<Set<string>>(new Set());
  const lastCueVersionBySender = useRef<Record<string, CuePacketVersion>>({});
  const clientSeqRef = useRef(0);
  const lastCueSentAtRef = useRef(0);
  const chatOpenRef = useRef(false);
  const lastSnapshotVersionRef = useRef<SnapshotVersion>({ shotSeq: -1, turnSeq: 0, updatedAtMs: 0 });
  const roomRef = useRef<Room | null>(null);
  const scoresRef = useRef<Scoreboard>(INITIAL_SCORES);
  const clockOffsetRef = useRef(0);
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
  const turnRemainingMs =
    turnDeadlineAtMs > 0 && hud.status === "aiming" && !hasReconnectingPlayer
      ? Math.max(0, turnDeadlineAtMs - nowMs)
      : 0;
  const turnExpired =
    !hasReconnectingPlayer && turnDeadlineAtMs > 0 && hud.status === "aiming" && nowMs >= turnDeadlineAtMs;
  const turnRemainingText =
    hasReconnectingPlayer && turnDeadlineAtMs > 0 && hud.status === "aiming"
      ? "Pausado"
      : turnDeadlineAtMs > 0 && hud.status === "aiming"
        ? `${Math.max(0, Math.ceil(turnRemainingMs / 1000))}s`
        : "--";

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
      turnSeq: snapshot.turn_seq,
      updatedAtMs: snapshot.updated_at_ms,
      status: snapshot.status
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
    const timer = window.setInterval(() => setNowMs(Date.now() - clockOffsetRef.current), 250);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    lastSnapshotVersionRef.current = { shotSeq: -1, turnSeq: 0, updatedAtMs: 0 };
    lastCueVersionBySender.current = {};
    clientSeqRef.current = 0;
    lastCueSentAtRef.current = 0;
    setRemoteCue(null);
    setTurnDeadlineAtMs(0);
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
      let opened = false;
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/rooms/${activeRoomId}/ws?token=${token}&client_id=${getRoomClientId()}`);
      wsRef.current = ws;

      ws.onopen = () => {
        opened = true;
        if (roomRef.current?.status === "playing") {
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
                setSpectatorList(snapshot.spectators || []);
                if (snapshot.spectators) {
                  snapshot.spectators.forEach((user) => {
                    ensureProfile(token, user.user_id);
                  });
                }
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
              const cue = rawPayload as CueStatePayload;
              if (!isUsableCuePayload(cue)) break;

              const nextVersion = cuePacketVersion(cue);
              const previousVersion = lastCueVersionBySender.current[senderId];
              if (!isNewerCuePacket(nextVersion, previousVersion)) break;

              lastCueVersionBySender.current[senderId] = nextVersion;
              setRemoteCue({
                senderId,
                client_seq: nextVersion.clientSeq,
                server_received_at_ms: nextVersion.serverReceivedAtMs,
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
              setRemoteCue(null);
              setTurnDeadlineAtMs(0);
              setIncomingShot(rawPayload as ShotStartedEvent);
              break;

            case "turn_timeout": {
              const timeoutPayload = rawPayload as TurnTimeoutPayload;
              setRemoteCue(null);
              setIncomingShot(null);
              if (timeoutPayload?.next_turn_user_id) {
                setTurnUserId(timeoutPayload.next_turn_user_id);
              }
              if (typeof timeoutPayload?.turn_deadline_at_ms === "number") {
                setTurnDeadlineAtMs(timeoutPayload.turn_deadline_at_ms);
              }
              if (typeof timeoutPayload?.turn_started_at_ms === "number") {
                const offset = Date.now() - timeoutPayload.turn_started_at_ms;
                clockOffsetRef.current = offset;
                setNowMs(Date.now() - offset);
              }
              break;
            }

            case "game_state_sync": {
              const snapshot = rawPayload as MatchSnapshot;
              if (!snapshot?.scores || !acceptSnapshot(snapshot)) {
                break;
              }
              if (snapshot?.updated_at_ms) {
                const offset = Date.now() - snapshot.updated_at_ms;
                clockOffsetRef.current = offset;
                setNowMs(Date.now() - offset);
                setIncomingSnapshot(snapshot);
              }
              setScores(snapshot.scores);
              setTurnDeadlineAtMs(
                snapshot.status === "aiming" && typeof snapshot.turn_deadline_at_ms === "number"
                  ? snapshot.turn_deadline_at_ms
                  : 0
              );
              if (snapshot?.turn_user_id) {
                setTurnUserId(snapshot.turn_user_id);
                setRemoteCue((current) =>
                  current && snapshot.status === "aiming" && current.turn_user_id === snapshot.turn_user_id
                    ? current
                    : null
                );
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
              setTurnDeadlineAtMs(0);
              setMatchSummary((prev) => ({
                winnerUserId: matchPayload.winner_user_id ?? prev?.winnerUserId,
                scores: matchPayload.scores ?? prev?.scores ?? scoresRef.current,
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
                setTurnDeadlineAtMs(0);
                lastCueVersionBySender.current = {};
                clientSeqRef.current = 0;
                lastCueSentAtRef.current = 0;
                lastSnapshotVersionRef.current = { shotSeq: -1, turnSeq: 0, updatedAtMs: 0 };
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
                setTurnDeadlineAtMs(0);
                lastCueVersionBySender.current = {};
                clientSeqRef.current = 0;
                lastCueSentAtRef.current = 0;
                lastSnapshotVersionRef.current = { shotSeq: -1, turnSeq: 0, updatedAtMs: 0 };
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

            case "room_entry_rejected":
              active = false;
              setError("Voce ja esta em uma partida ativa.");
              ws?.close();
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
        if (!opened) {
          active = false;
          setError("Nao foi possivel abrir esta partida. Verifique se ela ja esta aberta em outra aba.");
          return;
        }
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

  const handleLocalShotStarted = useCallback((shot: ShotStartedEvent) => {
    sendWS("shot_started", shot);
  }, [sendWS]);

  const handleShotResult = useCallback((result: ShotResultSubmittedEvent) => {
    if (!room) return;
    sendWS("shot_result_submitted", {
      ...result,
      match_id: room.id
    });
  }, [room?.id, sendWS]);

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
        <Button onClick={() => navigate("/")} className="mt-8 !w-auto px-6">
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
          disabled={hasReconnectingPlayer || room.status === "finished" || turnExpired}
          resetKey={resetKey}
          onCueState={sendCueState}
          onLocalShotStarted={handleLocalShotStarted}
          onShotResult={handleShotResult}
          onHudChange={setHud}
        />
      </section>

      <header className="pointer-events-none absolute inset-x-0 top-0 z-30 p-4 animate-fade-in flex justify-center">
        {/* Placar Flutuante Centrado */}
        <div className="pointer-events-auto flex items-center justify-between gap-6 bg-zinc-950/90 border border-white/10 rounded-2xl px-6 py-3 shadow-2xl backdrop-blur-xl max-w-2xl">
          {/* Jogador 1 (Dono) */}
          <div className="flex items-center gap-3">
            {/* Foto com Temporizador Borda */}
            <div className="relative h-14 w-14 shrink-0">
              {turnUserId === room.creator_id && turnRemainingMs > 0 && (
                <svg className="absolute -inset-1 h-16 w-16 -rotate-90 pointer-events-none" viewBox="0 0 64 64">
                  <rect
                    x="2"
                    y="2"
                    width="60"
                    height="60"
                    rx="12"
                    className="stroke-amber-400 transition-all duration-100 ease-linear"
                    strokeWidth="3.5"
                    fill="none"
                    strokeDasharray="240"
                    strokeDashoffset={240 * (1 - turnRemainingMs / 20000)}
                  />
                </svg>
              )}
              <div className={`h-full w-full overflow-hidden rounded-xl border-2 ${
                turnUserId === room.creator_id 
                  ? "border-amber-400 shadow-[0_0_12px_rgba(245,158,11,0.4)]" 
                  : "border-amber-450/40"
              } bg-zinc-900`}>
                {profilesById[room.creator_id]?.photo_url ? (
                  <img src={profilesById[room.creator_id].photo_url} alt="" className="h-full w-full object-cover" />
                ) : (
                  <div className="flex h-full w-full items-center justify-center font-bold text-lg bg-amber-500 text-white">
                    {creatorName.substring(0, 2).toUpperCase()}
                  </div>
                )}
              </div>
            </div>

            <div className="text-left max-w-[120px] md:max-w-[150px]">
              <div className="flex items-center gap-1.5">
                <span className="h-2.5 w-2.5 rounded-full bg-amber-400 shadow-[0_0_6px_rgba(245,158,11,0.8)] shrink-0" title="Amarelos" />
                <p className="text-sm font-black text-white truncate">{creatorName}</p>
              </div>
              {creatorDisconnected && (
                <span className="text-[9px] font-bold text-sky-400 animate-pulse block mt-0.5">Reconectando</span>
              )}
            </div>

            <span className="text-3xl font-black text-white tracking-tighter ml-2 bg-white/5 border border-white/10 px-3 py-1 rounded-lg min-w-10 text-center">
              {scores.creator}
            </span>
          </div>

          {/* Divisor VS */}
          <div className="text-[10px] font-black text-neutral-500 tracking-widest px-1 uppercase">VS</div>

          {/* Jogador 2 (Oponente) */}
          <div className="flex items-center gap-3">
            <span className="text-3xl font-black text-white tracking-tighter mr-2 bg-white/5 border border-white/10 px-3 py-1 rounded-lg min-w-10 text-center">
              {scores.opponent}
            </span>

            <div className="text-right max-w-[120px] md:max-w-[150px]">
              <div className="flex items-center justify-end gap-1.5">
                <p className="text-sm font-black text-white truncate">{opponentName}</p>
                <span className="h-2.5 w-2.5 rounded-full bg-sky-500 shadow-[0_0_6px_rgba(14,165,233,0.8)] shrink-0" title="Azuis" />
              </div>
              {opponentDisconnected && (
                <span className="text-[9px] font-bold text-sky-400 animate-pulse block mt-0.5">Reconectando</span>
              )}
            </div>

            {/* Foto com Temporizador Borda */}
            <div className="relative h-14 w-14 shrink-0">
              {room.opponent_id && turnUserId === room.opponent_id && turnRemainingMs > 0 && (
                <svg className="absolute -inset-1 h-16 w-16 -rotate-90 pointer-events-none" viewBox="0 0 64 64">
                  <rect
                    x="2"
                    y="2"
                    width="60"
                    height="60"
                    rx="12"
                    className="stroke-sky-400 transition-all duration-100 ease-linear"
                    strokeWidth="3.5"
                    fill="none"
                    strokeDasharray="240"
                    strokeDashoffset={240 * (1 - turnRemainingMs / 20000)}
                  />
                </svg>
              )}
              <div className={`h-full w-full overflow-hidden rounded-xl border-2 ${
                room.opponent_id 
                  ? turnUserId === room.opponent_id 
                    ? "border-sky-500 shadow-[0_0_12px_rgba(14,165,233,0.4)]" 
                    : "border-sky-550/40"
                  : "border-neutral-750 border-dashed"
              } bg-zinc-900`}>
                {room.opponent_id ? (
                  profilesById[room.opponent_id]?.photo_url ? (
                    <img src={profilesById[room.opponent_id].photo_url} alt="" className="h-full w-full object-cover" />
                  ) : (
                    <div className="flex h-full w-full items-center justify-center font-bold text-lg bg-sky-500 text-white">
                      {opponentName.substring(0, 2).toUpperCase()}
                    </div>
                  )
                ) : (
                  <div className="flex h-full w-full items-center justify-center text-neutral-600">
                    <span className="text-xl font-bold">+</span>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </header>

      {/* Botões de Ação de HUD (Chat e Espectadores) */}
      <div className="absolute top-4 right-4 z-30 flex gap-2">
        {/* Espectadores */}
        <button
          type="button"
          onClick={() => {
            setSpectatorsOpen((open) => !open);
            setChatOpen(false);
          }}
          className="h-12 px-4 rounded-xl border border-white/10 bg-zinc-950/85 hover:bg-zinc-900 text-white font-bold transition flex items-center gap-2 shadow-lg backdrop-blur-md"
          title="Ver espectadores"
        >
          <svg className="h-5 w-5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            <path strokeLinecap="round" strokeLinejoin="round" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
          </svg>
          <span className="text-sm font-black">{spectators}</span>
        </button>

        {/* Chat */}
        <button
          type="button"
          onClick={() => {
            setChatOpen((open) => !open);
            setSpectatorsOpen(false);
            setUnreadMessages(0);
          }}
          className="relative h-12 w-12 rounded-xl border border-white/10 bg-zinc-950/85 hover:bg-zinc-900 text-white transition flex items-center justify-center shadow-lg backdrop-blur-md"
          title="Ver chat"
        >
          <svg className="h-5 w-5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
          </svg>
          {unreadMessages > 0 && !chatOpen && (
            <span className="absolute -top-1.5 -right-1.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-red-600 px-1 text-[10px] font-black text-white shadow-md animate-pulse">
              {unreadMessages}
            </span>
          )}
        </button>
      </div>

      {hasReconnectingPlayer && (
        <div className="pointer-events-none absolute left-1/2 top-36 z-30 w-[min(92vw,720px)] -translate-x-1/2 border border-amber-400/20 bg-amber-500/10 px-4 py-3 rounded-xl text-xs font-semibold text-amber-300 shadow-xl backdrop-blur-md text-center animate-pulse">
          {creatorDisconnected && opponentDisconnected
            ? "Os dois jogadores estão reconectando. A partida fica pausada para manter a simulação consistente."
            : `${creatorDisconnected ? creatorName : opponentName} está reconectando. A sala permanece viva enquanto aguardamos o retorno.`}
        </div>
      )}

      {/* Drawer do Chat */}
      <aside
        className={`fixed right-4 top-4 bottom-4 z-50 flex w-[min(90vw,360px)] flex-col overflow-hidden rounded-2xl border border-white/10 bg-zinc-950/95 shadow-2xl backdrop-blur-xl transition-all duration-300 transform ${
          chatOpen ? "translate-x-0 opacity-100" : "pointer-events-none translate-x-[calc(100%+2rem)] opacity-0"
        }`}
      >
        <header className="flex items-center justify-between border-b border-white/10 px-4 py-3 bg-zinc-950/40">
          <h2 className="text-sm font-bold uppercase tracking-[0.2em] text-neutral-300">Chat da Partida</h2>
          <button
            type="button"
            onClick={() => setChatOpen(false)}
            className="h-8 w-8 rounded-lg border border-white/5 text-sm text-neutral-400 transition hover:border-white/20 hover:text-white flex items-center justify-center"
            aria-label="Fechar chat"
          >
            ×
          </button>
        </header>

        <div className="flex-1 space-y-3 overflow-y-auto p-4 scrollbar-thin">
          {messages.length === 0 ? (
            <div className="flex h-full items-center justify-center text-center text-xs text-neutral-600 font-medium">
              Nenhuma mensagem nesta partida.
            </div>
          ) : (
            messages.map((message) => (
              <div key={message.messageId} className={`flex w-full min-w-0 flex-col ${message.senderId === userId ? "items-end" : "items-start"}`}>
                <span className="mb-0.5 max-w-[86%] truncate px-1 text-[10px] text-neutral-500 font-bold">{message.senderName}</span>
                <div className={`max-w-[86%] overflow-hidden break-words px-3.5 py-1.5 text-sm [overflow-wrap:anywhere] border ${
                  message.senderId === userId
                    ? "bg-red-500/10 border-red-500/20 dark:bg-red-600/20 dark:border-red-500/20 text-red-100 rounded-xl rounded-tr-none"
                    : "bg-white/[0.04] border-white/5 text-neutral-200 rounded-xl rounded-tl-none"
                }`}>
                  {message.text}
                </div>
              </div>
            ))
          )}
          <div ref={chatEndRef} />
        </div>

        <form onSubmit={handleSendChat} className="flex gap-2 border-t border-white/10 p-3 bg-zinc-950/20">
          <input
            type="text"
            value={messageText}
            onChange={(event) => setMessageText(event.target.value)}
            disabled={!isParticipant}
            placeholder={isParticipant ? "Mensagem..." : "Apenas leitura"}
            className="min-w-0 flex-1 rounded-lg border border-white/10 bg-neutral-950 px-3 py-2 text-sm text-white placeholder-neutral-600 outline-none focus:border-red-500/60 focus:ring-2 focus:ring-red-500/10 disabled:opacity-50 transition-all"
          />
          <button
            type="submit"
            disabled={!isParticipant || !messageText.trim()}
            className="flex items-center justify-center h-10 w-10 shrink-0 rounded-lg bg-red-600 hover:bg-red-500 disabled:bg-neutral-800 disabled:text-neutral-600 text-white transition active:scale-95 shadow-md"
            aria-label="Enviar mensagem"
          >
            <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5" />
            </svg>
          </button>
        </form>
      </aside>

      {/* Drawer de Espectadores */}
      <aside
        className={`fixed right-4 top-4 bottom-4 z-50 flex w-[min(90vw,320px)] flex-col overflow-hidden rounded-2xl border border-white/10 bg-zinc-950/95 shadow-2xl backdrop-blur-xl transition-all duration-300 transform ${
          spectatorsOpen ? "translate-x-0 opacity-100" : "pointer-events-none translate-x-[calc(100%+2rem)] opacity-0"
        }`}
      >
        <header className="flex items-center justify-between border-b border-white/10 px-4 py-3 bg-zinc-950/40">
          <h2 className="text-sm font-bold uppercase tracking-[0.2em] text-neutral-300">Espectadores ({spectators})</h2>
          <button
            type="button"
            onClick={() => setSpectatorsOpen(false)}
            className="h-8 w-8 rounded-lg border border-white/5 text-sm text-neutral-400 transition hover:border-white/20 hover:text-white flex items-center justify-center"
            aria-label="Fechar espectadores"
          >
            ×
          </button>
        </header>

        <div className="flex-1 space-y-3 overflow-y-auto p-4 scrollbar-thin">
          {spectatorList.length === 0 ? (
            <div className="flex h-full items-center justify-center text-center text-xs text-neutral-600 font-medium">
              Nenhum espectador assistindo.
            </div>
          ) : (
            spectatorList.map((spec) => {
              const specName = profilesById[spec.user_id]?.nickname ?? "Carregando...";
              const specPhoto = profilesById[spec.user_id]?.photo_url;
              return (
                <div key={spec.user_id} className="flex items-center gap-3 rounded-xl bg-white/[0.02] border border-white/5 px-3 py-2">
                  <div className="h-8 w-8 rounded-lg overflow-hidden border border-white/10 bg-zinc-800 shrink-0">
                    {specPhoto ? (
                      <img src={specPhoto} alt="" className="h-full w-full object-cover" />
                    ) : (
                      <div className="flex h-full w-full items-center justify-center font-bold text-xs bg-neutral-700 text-white">
                        {specName.substring(0, 2).toUpperCase()}
                      </div>
                    )}
                  </div>
                  <span className="text-sm font-semibold text-neutral-250 truncate">{specName}</span>
                </div>
              );
            })
          )}
        </div>
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
  const ballStyle = tone === "creator" 
    ? "bg-gradient-to-br from-yellow-300 via-amber-400 to-amber-600 shadow-[inset_-2px_-2px_4px_rgba(0,0,0,0.5),_0_0_8px_rgba(245,158,11,0.4)]" 
    : "bg-gradient-to-br from-sky-300 via-sky-500 to-blue-700 shadow-[inset_-2px_-2px_4px_rgba(0,0,0,0.5),_0_0_8px_rgba(14,165,233,0.4)]";

  return (
    <div className={`rounded-xl border p-3.5 transition-all duration-300 shadow-lg ${
      active 
        ? "border-red-500 bg-red-50 dark:bg-red-950/20 shadow-[0_0_12px_rgba(239,68,68,0.1)] dark:shadow-[0_0_12px_rgba(239,68,68,0.15)]" 
        : "border-neutral-200 dark:border-white/5 bg-neutral-50 dark:bg-zinc-900/40"
    }`}>
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <span className={`h-3.5 w-3.5 rounded-full ${ballStyle}`} />
          <span className="truncate text-[9px] uppercase tracking-[0.2em] text-neutral-550 dark:text-neutral-500 font-bold">{label}</span>
        </div>
        <span className={`text-[9px] font-bold tracking-wider uppercase ${
          reconnecting ? "text-amber-400 animate-pulse" : active ? "text-red-600 dark:text-red-400 animate-pulse" : "text-neutral-500"
        }`}>
          {reconnecting ? "Recon." : active ? "Turno" : xpAward ? `+${xpAward.xp_delta} XP` : "Online"}
        </span>
      </div>
      <p className="mt-2 truncate text-sm font-extrabold text-neutral-800 dark:text-white">{name}</p>
      <div className="mt-1 flex items-baseline justify-between">
        <strong className="text-3xl font-black leading-none text-neutral-900 dark:text-white tracking-tighter">{score}</strong>
      </div>
    </div>
  );
}

function StatusPill({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-24 rounded-lg border border-neutral-200 dark:border-white/5 bg-white/95 dark:bg-zinc-905/90 px-3.5 py-2.5 backdrop-blur-md shadow-lg">
      <p className="text-[9px] uppercase tracking-[0.2em] font-bold text-neutral-550 dark:text-neutral-500">{label}</p>
      <p className="truncate text-xs font-black tracking-wide text-neutral-900 dark:text-white mt-0.5">{value}</p>
    </div>
  );
}

type XPAwardType = {
  user_id: string;
  xp_delta: number;
};

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
  xpAwards: XPAwardType[];
  profilesById: Record<string, Profile>;
  onRequestRematch: () => void;
  onCloseRoom: () => void;
  onLeave: () => void;
  onLobby: () => void;
}) {
  return (
    <div className="absolute inset-0 z-50 flex items-center justify-center bg-neutral-950/80 p-4 backdrop-blur-md animate-fade-in">
      <section className="w-full max-w-lg rounded-2xl border border-neutral-200 dark:border-white/10 bg-white dark:bg-zinc-950/95 p-8 text-neutral-900 dark:text-white shadow-2xl shadow-neutral-200/50 dark:shadow-black/90">
        <div className="text-center mb-6">
          <p className="text-xs uppercase tracking-[0.25em] text-red-600 dark:text-red-400 font-bold">Partida Finalizada</p>
          <h2 className="mt-2 text-3xl font-black tracking-tight text-neutral-900 dark:text-white">
            {winnerName ? `${winnerName} Venceu!` : "Resultado Registrado"}
          </h2>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <ResultScore name={creatorName} label="Amarelos" score={scores.creator} />
          <ResultScore name={opponentName} label="Azuis" score={scores.opponent} />
        </div>

        <div className="mt-6 rounded-xl border border-neutral-200 dark:border-white/5 bg-neutral-50 dark:bg-white/[0.02] p-4 shadow-inner">
          <p className="text-[10px] uppercase tracking-[0.2em] text-neutral-550 dark:text-neutral-500 font-bold border-b border-neutral-200 dark:border-white/5 pb-2 mb-3">Recompensas de XP</p>
          <div className="space-y-2">
            {xpAwards.length === 0 ? (
              <p className="text-xs text-neutral-550 dark:text-neutral-500 italic">Aguardando confirmação do servidor...</p>
            ) : (
              xpAwards.map((award) => (
                <div key={award.user_id} className="flex items-center justify-between text-sm">
                  <span className="truncate text-neutral-700 dark:text-neutral-300 font-medium">{profilesById[award.user_id]?.nickname ?? "Jogador"}</span>
                  <strong className="text-red-650 dark:text-red-450 font-bold">+{award.xp_delta} XP</strong>
                </div>
              ))
            )}
          </div>
        </div>

        <div className="mt-8 grid gap-3 sm:grid-cols-2">
          {isParticipant && (
            <Button onClick={onRequestRematch} disabled={currentUserRequestedRematch}>
              {currentUserRequestedRematch ? "Revanche solicitada" : "Pedir Revanche"}
            </Button>
          )}
          {isCreator ? (
            <Button variant="outline" onClick={onCloseRoom}>
              Encerrar Sala
            </Button>
          ) : (
            <Button variant="outline" onClick={onLeave}>
              Sair da Sala
            </Button>
          )}
          <Button variant="outline" onClick={onLobby} className="sm:col-span-2">
            Voltar ao Lobby
          </Button>
        </div>
      </section>
    </div>
  );
}

function ResultScore({ name, label, score }: { name: string; label: string; score: number }) {
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-white/5 bg-neutral-50 dark:bg-zinc-900/40 p-4 shadow-inner text-center">
      <p className="truncate text-sm font-bold text-neutral-800 dark:text-white">{name}</p>
      <p className="text-[9px] uppercase tracking-[0.2em] text-neutral-550 dark:text-neutral-500 font-bold mt-1">{label}</p>
      <strong className="mt-3 block text-4xl font-black leading-none text-neutral-900 dark:text-white tracking-tighter">{score}</strong>
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
      return "Rolando";
    default:
      return "Finalizada";
  }
}

