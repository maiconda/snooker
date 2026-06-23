import { useEffect, useRef, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import { useTheme } from "../components/ThemeContext";
import { getRoom, inviteUser, leaveRoom } from "../lobby/lobbyApi";
import { getRoomClientId } from "../lobby/roomClient";
import { useLobbyNotifications } from "../lobby/LobbyNotificationsProvider";
import type {
  MatchFinishedPayload,
  PresenceUser,
  RematchRequestedPayload,
  Room,
  RoomSpectatorsSnapshot,
  WSEvent
} from "../lobby/types";
import { getPublicProfile } from "../profile/profileApi";
import type { Profile } from "../profile/types";
import { navigate } from "../lib/router";

type ChatMessage = {
  messageId: string;
  senderId: string;
  senderName: string;
  text: string;
  timestamp: number;
};

type RoomEventPayload = {
  room?: Room;
  message_id?: string;
  text?: string;
  ready?: boolean;
  created_at?: string;
  reason?: string;
};

export function LobbyRoomPage({ roomId }: { roomId: string }) {
  const { session } = useAuth();
  const { theme } = useTheme();
  const [room, setRoom] = useState<Room | null>(null);
  const [creatorProfile, setCreatorProfile] = useState<Profile | null>(null);
  const [opponentProfile, setOpponentProfile] = useState<Profile | null>(null);
  const [profilesById, setProfilesById] = useState<Record<string, Profile>>({});
  const [spectators, setSpectators] = useState<PresenceUser[]>([]);
  const [inviteStatuses, setInviteStatuses] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Estados de prontidão locais
  const [creatorReady, setCreatorReady] = useState(false);
  const [opponentReady, setOpponentReady] = useState(false);
  const [rematchRequestedBy, setRematchRequestedBy] = useState<Set<string>>(new Set());

  // Chat
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [messageText, setMessageText] = useState("");
  const chatContainerRef = useRef<HTMLDivElement>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const { onlineUsers, clearedInvites } = useLobbyNotifications();
  const profilesCache = useRef<Record<string, Profile>>({});
  const chatMessageIdsRef = useRef<Set<string>>(new Set());
  const roomRef = useRef<Room | null>(null);
  roomRef.current = room;

  const rememberProfile = (userId: string, profile: Profile) => {
    setProfilesById((prev) => prev[userId] ? prev : { ...prev, [userId]: profile });
  };

  const ensureProfile = async (token: string, userId: string, isActive: () => boolean = () => true) => {
    if (!profilesCache.current[userId]) {
      const profile = await getPublicProfile(token, userId);
      if (!isActive()) return undefined;
      profilesCache.current[userId] = profile;
    }
    const profile = profilesCache.current[userId];
    rememberProfile(userId, profile);
    return profile;
  };

  const hydrateRoomProfiles = async (roomData: Room, token: string, isActive: () => boolean = () => true) => {
    const creator = await ensureProfile(token, roomData.creator_id, isActive);
    if (!creator || !isActive()) return;
    setCreatorProfile(creator);

    if (roomData.opponent_id) {
      const opponent = await ensureProfile(token, roomData.opponent_id, isActive);
      if (!opponent || !isActive()) return;
      setOpponentProfile(opponent);
    } else {
      setOpponentProfile(null);
    }
  };

  // Carregar dados iniciais da sala e perfis
  useEffect(() => {
    let active = true;
    const token = session?.accessToken;
    if (!token) return;
    setMessages([]);
    chatMessageIdsRef.current.clear();

    async function loadData() {
      const activeToken = session?.accessToken;
      if (!activeToken || !roomId) return;
      try {
        setLoading(true);
        // Obter detalhes da sala
        const roomData = await getRoom(activeToken, roomId);
        if (!active) return;
        setRoom(roomData);
        await hydrateRoomProfiles(roomData, activeToken, () => active);
        if (roomData.status === "playing") {
          navigate(`/jogar/${roomData.id}`);
          return;
        }
      } catch (err) {
        if (active) {
          setError(err instanceof Error ? err.message : "Falha ao carregar a sala.");
        }
      } finally {
        if (active) setLoading(false);
      }
    }

    loadData();
    return () => {
      active = false;
    };
  }, [roomId, session?.accessToken]);

  useEffect(() => {
    let active = true;
    const token = session?.accessToken;
    if (!token) return;

    spectators.forEach((spectator) => {
      void ensureProfile(token, spectator.user_id, () => active).catch((err) => {
        console.error("Erro ao carregar perfil do espectador:", err);
      });
    });

    return () => {
      active = false;
    };
  }, [spectators, session?.accessToken]);

  useEffect(() => {
    let active = true;
    const token = session?.accessToken;
    if (!token) return;

    onlineUsers.forEach((user) => {
      void ensureProfile(token, user.user_id, () => active).catch((err) => {
        console.error("Erro ao carregar perfil online:", err);
      });
    });

    return () => {
      active = false;
    };
  }, [onlineUsers, session?.accessToken]);

  useEffect(() => {
    if (!room) return;
    setInviteStatuses((prev) => {
      let changed = false;
      const next = { ...prev };
      clearedInvites.forEach((cleared) => {
        if (cleared.room_id !== room.id) return;
        if (next[cleared.to_user_id] || Object.values(next).includes(cleared.invitation_id)) {
          delete next[cleared.to_user_id];
          changed = true;
        }
      });
      return changed ? next : prev;
    });
  }, [clearedInvites, room]);

  // Conectar WebSocket
  useEffect(() => {
    const token = session?.accessToken;
    const activeRoomId = room?.id;
    if (!token || !activeRoomId) return;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/api/v1/rooms/${activeRoomId}/ws?token=${token}&client_id=${getRoomClientId()}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onmessage = async (event) => {
      const currentRoom = roomRef.current;
      if (!currentRoom) return;

      try {
        const wsMsg = JSON.parse(event.data) as WSEvent;
        const senderId = wsMsg.sender_id ?? "";

        // Carregar perfil se não estiver em cache
        let senderName = "Jogador";
        if (senderId) {
          if (!profilesCache.current[senderId]) {
            try {
              const p = await getPublicProfile(token, senderId);
              profilesCache.current[senderId] = p;
            } catch (err) {
              console.error("Erro ao carregar perfil do remetente:", err);
            }
          }
          senderName = profilesCache.current[senderId]?.nickname ?? "Jogador";
        }

        const rawPayload = wsMsg.payload as unknown;
        const payload = (rawPayload && typeof rawPayload === "object" ? rawPayload : {}) as RoomEventPayload;
        const roomFromPayload = rawPayload && typeof rawPayload === "object" && "id" in rawPayload
          ? (rawPayload as Room)
          : payload.room;
        const applyRoomUpdate = async (nextRoom?: Room) => {
          const updatedRoom = nextRoom ?? await getRoom(token, currentRoom.id);
          setRoom(updatedRoom);
          await hydrateRoomProfiles(updatedRoom, token);
          if (updatedRoom.status === "playing") {
            navigate(`/jogar/${updatedRoom.id}`);
          }
        };

        switch (wsMsg.type) {
          case "room_state":
          case "player_joined":
            setInviteStatuses({});
            setRematchRequestedBy(new Set());
            setError(null);
            await applyRoomUpdate(roomFromPayload);
            break;

          case "owner_reconnected":
          case "player_reconnected":
            setError(null);
            await applyRoomUpdate(roomFromPayload);
            break;

          case "owner_disconnected":
            setRoom((prev) => prev ? { ...prev, creator_disconnected_at: new Date().toISOString() } : prev);
            break;

          case "player_disconnected":
            if (senderId !== currentRoom.creator_id) {
              setOpponentReady(false);
              setRoom((prev) => prev ? { ...prev, opponent_disconnected_at: new Date().toISOString() } : prev);
            }
            break;

          case "player_left":
            setInviteStatuses({});
            setRematchRequestedBy(new Set());
            await applyRoomUpdate(roomFromPayload);
            if (senderId !== currentRoom.creator_id) {
              setOpponentReady(false);
            }
            break;

          case "room_spectators_snapshot":
            if (rawPayload && typeof rawPayload === "object" && "spectators" in rawPayload) {
              const snapshot = rawPayload as RoomSpectatorsSnapshot;
              setSpectators(Array.isArray(snapshot.spectators) ? snapshot.spectators : []);
            }
            break;

          case "chat_message":
            const chatText = payload.text;
            if (!chatText) break;
            const createdAt = payload.created_at ?? "";
            const messageId = payload.message_id ?? `${senderId}:${createdAt}:${chatText}`;
            if (chatMessageIdsRef.current.has(messageId)) break;
            chatMessageIdsRef.current.add(messageId);
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
            break;

          case "player_ready":
            if (typeof payload.ready !== "boolean") break;
            if (senderId === currentRoom.creator_id) {
              setCreatorReady(payload.ready);
            } else {
              setOpponentReady(payload.ready);
            }
            break;

          case "match_start":
            if (roomFromPayload) {
              setRoom(roomFromPayload);
            }
            setRematchRequestedBy(new Set());
            // Redireciona para o Game Core
            navigate(`/jogar/${currentRoom.id}`, { isNewMatch: true });
            break;

          case "match_finished": {
            const matchPayload = payload as MatchFinishedPayload;
            if (matchPayload.room) {
              setRoom(matchPayload.room);
            }
            setCreatorReady(false);
            setOpponentReady(false);
            setRematchRequestedBy(new Set());
            navigate(`/sala/${currentRoom.id}`);
            break;
          }

          case "rematch_requested": {
            const rematchPayload = payload as RematchRequestedPayload;
            const requested = rematchPayload.requested_user_ids?.length
              ? rematchPayload.requested_user_ids
              : rematchPayload.user_id
                ? [rematchPayload.user_id]
                : [];
            setRematchRequestedBy((prev) => {
              const next = new Set(prev);
              requested.forEach((requestUserId) => next.add(requestUserId));
              return next;
            });
            break;
          }

          case "room_reset":
            setCreatorReady(false);
            setOpponentReady(false);
            setRematchRequestedBy(new Set());
            setError(null);
            await applyRoomUpdate(roomFromPayload);
            break;

          case "room_closed":
            setError(payload.reason === "owner_left"
              ? "O dono saiu da sala. A sala foi encerrada."
              : payload.reason === "owner_closed"
                ? "O dono encerrou a sala."
                : "O dono ficou desconectado por muito tempo. A sala foi encerrada.");
            navigate("/");
            break;

          default:
            break;
        }
      } catch (err) {
        console.error("Erro ao processar mensagem WebSocket:", err);
      }
    };

    return () => {
      ws.close();
    };
  }, [room?.id, session?.accessToken]);

  // Rolar chat para o final (apenas se houver mensagens)
  useEffect(() => {
    if (messages.length > 0 && chatContainerRef.current) {
      chatContainerRef.current.scrollTop = chatContainerRef.current.scrollHeight;
    }
  }, [messages]);

  const handleSendChat = (e: React.FormEvent) => {
    e.preventDefault();
    const isParticipant = Boolean(room && (session?.userId === room.creator_id || session?.userId === room.opponent_id));
    if (!isParticipant || !messageText.trim() || !wsRef.current) return;

    const eventMsg = {
      type: "chat_message",
      payload: { text: messageText }
    };
    wsRef.current.send(JSON.stringify(eventMsg));
    setMessageText("");
  };

  const handleToggleReady = () => {
    if (!wsRef.current || !room) return;
    const isCreator = session?.userId === room.creator_id;
    const isOpponent = session?.userId === room.opponent_id;
    if (!isCreator && !isOpponent) return;
    const currentReady = isCreator ? creatorReady : opponentReady;
    const nextReady = !currentReady;

    // Atualização otimista imediata na UI
    if (isCreator) {
      setCreatorReady(nextReady);
    } else {
      setOpponentReady(nextReady);
    }

    const eventMsg = {
      type: "player_ready",
      payload: { ready: nextReady }
    };
    wsRef.current.send(JSON.stringify(eventMsg));
  };

  const handleStartMatch = () => {
    if (!wsRef.current || !room || session?.userId !== room.creator_id) return;
    const eventMsg = {
      type: "match_start"
    };
    wsRef.current.send(JSON.stringify(eventMsg));
  };

  const handleRequestRematch = () => {
    if (!wsRef.current || !room) return;
    const isCreator = session?.userId === room.creator_id;
    const isOpponent = session?.userId === room.opponent_id;
    if (!isCreator && !isOpponent) return;

    wsRef.current.send(JSON.stringify({ type: "rematch_request" }));
    const requestedUserId = session?.userId;
    if (requestedUserId) {
      setRematchRequestedBy((prev) => new Set(prev).add(requestedUserId));
    }
  };

  const handleCloseRoom = () => {
    if (!wsRef.current || !room || session?.userId !== room.creator_id) return;
    wsRef.current.send(JSON.stringify({ type: "room_close_request" }));
  };

  const handleLeaveRoom = async () => {
    const token = session?.accessToken;
    if (!token || !room) {
      navigate("/");
      return;
    }

    const isParticipant = session?.userId === room.creator_id || session?.userId === room.opponent_id;
    if (!isParticipant) {
      navigate("/");
      return;
    }

    try {
      await leaveRoom(token, room.id);
      navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Falha ao sair da sala.");
    }
  };

  const handleInviteUser = async (userId: string) => {
    const token = session?.accessToken;
    if (!token || !room) return;

    setInviteStatuses((prev) => ({ ...prev, [userId]: "Enviando..." }));
    try {
      const invite = await inviteUser(token, room.id, userId);
      setInviteStatuses((prev) => ({ ...prev, [userId]: invite.invitation_id }));
    } catch (err) {
      setInviteStatuses((prev) => ({ ...prev, [userId]: err instanceof Error ? err.message : "Falha ao convidar" }));
    }
  };

  if (loading) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-neutral-950 text-white">
        <p className="text-sm text-neutral-400">Carregando sala...</p>
      </main>
    );
  }

  if (error || !room) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center bg-neutral-950 px-6 text-white">
        <h1 className="text-xl font-semibold text-red-500">Erro na Sala</h1>
        <p className="mt-2 text-sm text-neutral-400">{error ?? "Sala inválida."}</p>
        <Button onClick={() => navigate("/")} className="mt-8">
          Voltar para Início
        </Button>
      </main>
    );
  }

  const isCreator = session?.userId === room.creator_id;
  const isOpponent = session?.userId === room.opponent_id;
  const isSpectator = !isCreator && !isOpponent;
  const creatorDisconnected = Boolean(room.creator_disconnected_at);
  const opponentDisconnected = Boolean(room.opponent_disconnected_at);
  const bothReady = !creatorDisconnected && !opponentDisconnected && creatorReady && opponentReady;
  const canStartMatch = room.status === "waiting" && bothReady;
  const roomHasInviteSlot = room.status === "waiting" && !room.opponent_id && !creatorDisconnected && !opponentDisconnected;
  const onlineInviteCandidates = onlineUsers.filter((user) =>
    user.user_id !== room.creator_id &&
    user.user_id !== room.opponent_id &&
    user.user_id !== session?.userId
  );
  const isFinished = room.status === "finished";
  const isPlaying = room.status === "playing";
  const hasRequestedRematch = Boolean(session?.userId && rematchRequestedBy.has(session.userId));
  const creatorRequestedRematch = rematchRequestedBy.has(room.creator_id);
  const opponentRequestedRematch = Boolean(room.opponent_id && rematchRequestedBy.has(room.opponent_id));

  return (
    <main className="relative min-h-screen bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-red-100/30 via-zinc-50 to-zinc-50 dark:from-red-950/20 dark:via-neutral-950 dark:to-neutral-950 p-6 text-neutral-900 dark:text-white transition-colors duration-300 animate-fade-in">
      <div className="absolute inset-0 bg-[linear-gradient(to_bottom,rgba(0,0,0,0.015)_1px,transparent_1px)] dark:bg-[linear-gradient(to_bottom,rgba(255,255,255,0.005)_1px,transparent_1px)] bg-[size:100%_40px] pointer-events-none" />
      <div className="relative mx-auto flex min-h-[calc(100vh-3rem)] max-w-5xl flex-col justify-center">
            <header className="mb-8 flex flex-col justify-between gap-4 border-b border-neutral-200 dark:border-white/10 pb-6 md:flex-row md:items-end">
          <div className="flex justify-between items-start w-full md:w-auto">
            <div>
              <span className="text-sm uppercase tracking-[0.2em] text-red-650 dark:text-red-400 font-bold">
                {room.is_private ? "Mesa Privada" : "Mesa Pública"}
              </span>
              <h1 className="mt-1 text-3xl font-bold tracking-tight text-neutral-900 dark:text-white">Sala de Espera</h1>
              <p className="text-base text-neutral-550 dark:text-neutral-400 mt-1">
                ID da partida: {room.id}{isSpectator ? " • Assistindo" : ""}
              </p>
            </div>
          </div>

          {room.code && (
            <div className="rounded-lg border border-neutral-200 dark:border-white/10 bg-white/80 dark:bg-zinc-900/60 p-4 backdrop-blur-sm md:text-right shadow-lg shadow-neutral-200/20 dark:shadow-black/20 self-start md:self-end">
              <span className="text-sm text-neutral-500 dark:text-neutral-400 uppercase tracking-wider block">Código para Amigos</span>
              <strong className="text-2xl font-mono text-red-650 dark:text-red-400 tracking-widest">{room.code}</strong>
            </div>
          )}
        </header>

        {/* Grade Principal */}
        <div className="grid gap-6 md:grid-cols-3">
          
          {/* Jogadores (2 colunas) */}
          <section className="grid gap-6 sm:grid-cols-2 md:col-span-2">
            
            {/* Criador */}
            <div className={`relative overflow-hidden rounded-xl border p-6 backdrop-blur-md flex flex-col justify-between min-h-[220px] transition-all duration-300 shadow-xl ${
              creatorReady && !creatorDisconnected
                ? "border-red-500/30 bg-red-50 dark:bg-red-950/10 shadow-[0_0_20px_rgba(239,68,68,0.05)]"
                : "border-neutral-200 dark:border-white/10 bg-white/40 dark:bg-zinc-900/30 hover:border-neutral-300 dark:hover:border-white/20"
            }`}>
              <div className="flex items-center gap-4">
                {creatorProfile?.photo_url ? (
                  <img
                    src={creatorProfile.photo_url}
                    alt={creatorProfile.nickname}
                    className="h-12 w-12 rounded-xl object-cover border border-neutral-200 dark:border-white/10 shadow-md"
                  />
                ) : (
                  <div className="h-12 w-12 rounded-xl bg-gradient-to-tr from-red-600 to-rose-500 flex items-center justify-center font-bold text-xl text-white shadow-md">
                    {creatorProfile?.nickname?.substring(0, 2).toUpperCase() ?? "CR"}
                  </div>
                )}
                <div>
                  <h3 className="font-extrabold text-xl text-neutral-800 dark:text-white">{creatorProfile?.nickname ?? "Carregando..."}</h3>
                  <span className="text-base text-neutral-500 dark:text-neutral-400 font-medium">Dono da Mesa • Nível {Math.floor((creatorProfile?.xp ?? 0) / 100) + 1}</span>
                </div>
              </div>
              
              <div className="mt-8 flex items-center justify-between">
                <span className="text-base text-neutral-500 dark:text-neutral-400 uppercase tracking-wider font-bold">Estado</span>
                <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-base font-bold transition-all ${
                  creatorDisconnected
                    ? "bg-sky-500/10 text-sky-600 dark:text-sky-300 border border-sky-500/20"
                    : creatorReady
                      ? "bg-red-500/10 text-red-650 dark:text-red-400 border border-red-500/20"
                      : "bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20"
                }`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${
                    creatorDisconnected ? "bg-sky-300" : creatorReady ? "bg-red-500 dark:bg-red-400" : "bg-amber-500"
                  }`} />
                  {creatorDisconnected ? "Reconectando" : creatorReady ? "Pronto" : "Aguardando"}
                </span>
              </div>
            </div>

            {/* Oponente */}
            <div className={`relative overflow-hidden rounded-xl border p-6 backdrop-blur-md flex flex-col justify-between min-h-[220px] transition-all duration-300 shadow-xl ${
              opponentReady && !opponentDisconnected
                ? "border-red-500/30 bg-red-50 dark:bg-red-950/10 shadow-[0_0_20px_rgba(239,68,68,0.05)]"
                : "border-neutral-200 dark:border-white/10 bg-white/40 dark:bg-zinc-900/30 hover:border-neutral-300 dark:hover:border-white/20"
            }`}>
              {opponentProfile ? (
                <>
                  <div className="flex items-center gap-4">
                    {opponentProfile?.photo_url ? (
                      <img
                        src={opponentProfile.photo_url}
                        alt={opponentProfile.nickname}
                        className="h-12 w-12 rounded-xl object-cover border border-neutral-200 dark:border-white/10 shadow-md"
                      />
                    ) : (
                      <div className="h-12 w-12 rounded-xl bg-gradient-to-tr from-sky-600 to-indigo-500 flex items-center justify-center font-bold text-xl text-white shadow-md">
                        {opponentProfile?.nickname?.substring(0, 2).toUpperCase() ?? "OP"}
                      </div>
                    )}
                    <div>
                      <h3 className="font-extrabold text-xl text-neutral-800 dark:text-white">{opponentProfile?.nickname}</h3>
                      <span className="text-base text-neutral-500 dark:text-neutral-400 font-medium">Oponente • Nível {Math.floor((opponentProfile?.xp ?? 0) / 100) + 1}</span>
                    </div>
                  </div>
                  
                  <div className="mt-8 flex items-center justify-between">
                    <span className="text-base text-neutral-500 dark:text-neutral-400 uppercase tracking-wider font-bold">Estado</span>
                    <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-base font-bold transition-all ${
                      opponentDisconnected
                        ? "bg-sky-500/10 text-sky-600 dark:text-sky-300 border border-sky-500/20"
                        : opponentReady
                          ? "bg-red-500/10 text-red-650 dark:text-red-400 border border-red-500/20"
                          : "bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20"
                    }`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${
                        opponentDisconnected ? "bg-sky-300" : opponentReady ? "bg-red-500 dark:bg-red-400" : "bg-amber-500"
                      }`} />
                      {opponentDisconnected ? "Reconectando" : opponentReady ? "Pronto" : "Aguardando"}
                    </span>
                  </div>
                </>
              ) : (
                <div className="flex flex-col items-center justify-center h-full text-center py-6">
                  <div className="h-10 w-10 rounded-xl border border-dashed border-neutral-300 dark:border-white/10 flex items-center justify-center mb-3">
                    <span className="text-neutral-550 dark:text-neutral-500 text-lg animate-pulse">+</span>
                  </div>
                  <h4 className="text-lg font-bold text-neutral-800 dark:text-neutral-200">Aguardando Oponente</h4>
                  <p className="text-base text-neutral-500 dark:text-neutral-400 mt-2 max-w-[200px] leading-relaxed">
                    {room.is_private ? "Compartilhe o código para jogar." : "Partida pública aberta."}
                  </p>
                </div>
              )}
            </div>
          </section>

          {/* Chat em tempo real */}
          <section className="rounded-xl border border-neutral-200 dark:border-white/10 bg-white/40 dark:bg-zinc-900/30 backdrop-blur-md flex flex-col h-[380px] md:h-[450px] shadow-xl shadow-neutral-200/10 dark:shadow-none overflow-hidden">
            <header className="px-4 py-3 border-b border-neutral-200 dark:border-white/10 flex items-center justify-between bg-neutral-50/50 dark:bg-zinc-950/20">
              <span className="text-base uppercase tracking-wider text-neutral-550 dark:text-neutral-400 font-bold">Mensagens</span>
              <span className="text-sm bg-neutral-200 dark:bg-white/10 border border-neutral-300 dark:border-white/10 px-2.5 py-0.5 rounded text-neutral-650 dark:text-neutral-400 font-bold">{messages.length}</span>
            </header>

            {/* Lista de Mensagens */}
            <div ref={chatContainerRef} className="flex-1 overflow-y-auto p-4 space-y-3 scrollbar-thin">
              {messages.length === 0 ? (
                <div className="h-full flex items-center justify-center text-center text-sm text-neutral-450 dark:text-neutral-650 font-semibold">
                  Diga olá no chat da partida!
                </div>
              ) : (
                messages.map((msg) => (
                  <div key={msg.messageId} className={`flex flex-col ${msg.senderId === session?.userId ? "items-end" : "items-start"}`}>
                    <span className="text-sm text-neutral-550 dark:text-neutral-400 mb-0.5 px-1 font-bold">{msg.senderName}</span>
                    <div className={`px-3 py-1.5 rounded-lg text-base max-w-[85%] break-words border ${
                      msg.senderId === session?.userId 
                        ? "bg-red-500/10 border-red-500/20 dark:bg-red-600/20 dark:border-red-500/20 text-red-950 dark:text-red-100 rounded-tr-none" 
                        : "bg-neutral-100 dark:bg-white/[0.04] border-neutral-200 dark:border-white/5 text-neutral-800 dark:text-neutral-200 rounded-tl-none"
                    }`}>
                      {msg.text}
                    </div>
                  </div>
                ))
              )}
            </div>

            {/* Input do Chat */}
            <form onSubmit={handleSendChat} className="p-3 border-t border-neutral-200 dark:border-white/10 flex gap-2 bg-neutral-50 dark:bg-zinc-950/10">
              <input
                type="text"
                value={messageText}
                onChange={(e) => setMessageText(e.target.value)}
                disabled={isSpectator}
                placeholder={isSpectator ? "Espectadores apenas assistem ao chat" : "Escreva uma mensagem..."}
                className="flex-1 rounded-lg bg-white dark:bg-neutral-950 border border-neutral-350 dark:border-white/10 px-3 py-2 text-base placeholder-neutral-450 dark:placeholder-neutral-600 focus:outline-none focus:border-red-500/60 focus:ring-2 focus:ring-red-500/10 text-neutral-900 dark:text-white transition-all"
              />
              <button
                type="submit"
                disabled={isSpectator || !messageText.trim()}
                className="flex items-center justify-center h-10 w-10 shrink-0 rounded-lg bg-red-600 hover:bg-red-500 disabled:bg-neutral-200 dark:disabled:bg-neutral-800 disabled:text-neutral-400 dark:disabled:text-neutral-605 text-white transition active:scale-95 shadow-md"
                aria-label="Enviar mensagem"
              >
                <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 12L3.269 3.126A59.768 59.768 0 0121.485 12 59.77 59.77 0 013.27 20.876L5.999 12zm0 0h7.5" />
                </svg>
              </button>
            </form>
          </section>

        </div>

        {/* Rodapé de Ações */}
        <div className="mt-6 grid gap-6 md:grid-cols-2">
          <section className="rounded-xl border border-neutral-200 dark:border-white/10 bg-white/40 dark:bg-zinc-900/30 p-5 backdrop-blur-md shadow-xl shadow-neutral-200/10 dark:shadow-none">
            <div className="flex items-center justify-between border-b border-neutral-100 dark:border-white/5 pb-2">
              <h2 className="text-lg font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-300">Espectadores</h2>
              <span className="rounded-full bg-neutral-200 dark:bg-white/10 px-2.5 py-0.5 text-base font-bold text-neutral-650 dark:text-neutral-400">{spectators.length}</span>
            </div>
            <div className="mt-4 space-y-2 max-h-[160px] overflow-y-auto pr-1">
              {spectators.length === 0 ? (
                <p className="text-base text-neutral-500 dark:text-neutral-605 font-medium">Ninguém assistindo no momento.</p>
              ) : (
                spectators.map((spectator) => (
                  <div key={spectator.user_id} className="flex items-center gap-3 rounded-lg bg-neutral-100/50 dark:bg-white/[0.02] border border-neutral-200 dark:border-white/5 px-3 py-2">
                    {profilesById[spectator.user_id]?.photo_url ? (
                      <img
                        src={profilesById[spectator.user_id]?.photo_url}
                        alt={profilesById[spectator.user_id]?.nickname ?? "Espectador"}
                        className="h-8 w-8 rounded-lg object-cover"
                      />
                    ) : (
                      <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-neutral-200 dark:bg-neutral-800 text-base font-semibold text-neutral-650 dark:text-neutral-300">
                        {(profilesById[spectator.user_id]?.nickname ?? "ES").substring(0, 2).toUpperCase()}
                      </div>
                    )}
                    <span className="text-base text-neutral-800 dark:text-neutral-200 font-semibold">{profilesById[spectator.user_id]?.nickname ?? "Carregando..."}</span>
                  </div>
                ))
              )}
            </div>
          </section>
 
          {isCreator && (
            <section className="rounded-xl border border-neutral-200 dark:border-white/10 bg-white/40 dark:bg-zinc-900/30 p-5 backdrop-blur-md shadow-xl shadow-neutral-200/10 dark:shadow-none">
              <div className="flex items-center justify-between border-b border-neutral-100 dark:border-white/5 pb-2">
                <h2 className="text-lg font-bold uppercase tracking-wider text-neutral-500 dark:text-neutral-300">Jogadores online</h2>
                <span className="rounded-full bg-neutral-200 dark:bg-white/10 px-2.5 py-0.5 text-base font-bold text-neutral-650 dark:text-neutral-400">{onlineInviteCandidates.length}</span>
              </div>
              <div className="mt-4 space-y-2 max-h-[260px] overflow-y-auto pr-1">
                {onlineInviteCandidates.length === 0 ? (
                  <p className="text-base text-neutral-550 dark:text-neutral-500 font-medium">Nenhum jogador online disponível para convite.</p>
                ) : (
                  onlineInviteCandidates.map((user) => {
                    const status = inviteStatuses[user.user_id];
                    const wasInvited = Boolean(status && status !== "Enviando..." && !status.startsWith("Falha") && !status.startsWith("Usuario"));
                    return (
                      <div key={user.user_id} className="flex flex-col gap-2 rounded-xl bg-neutral-100/50 dark:bg-white/[0.02] border border-neutral-200 dark:border-white/5 p-3 shadow-sm">
                        <div className="flex min-w-0 items-center gap-3">
                          {profilesById[user.user_id]?.photo_url ? (
                            <img
                              src={profilesById[user.user_id]?.photo_url}
                              alt={profilesById[user.user_id]?.nickname ?? "Jogador"}
                              className="h-8 w-8 rounded-lg object-cover shrink-0"
                            />
                          ) : (
                            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-neutral-200 dark:bg-neutral-800 text-xs font-bold text-neutral-650 dark:text-neutral-300 shrink-0">
                              {(profilesById[user.user_id]?.nickname ?? "ON").substring(0, 2).toUpperCase()}
                            </div>
                          )}
                          <div className="min-w-0 flex-1">
                            <p className="truncate text-base text-neutral-800 dark:text-neutral-200 font-semibold" title={profilesById[user.user_id]?.nickname}>
                              {profilesById[user.user_id]?.nickname ?? "Carregando..."}
                            </p>
                            {status && !wasInvited && status !== "Enviando..." && (
                              <p className="truncate text-xs text-red-500 dark:text-red-405">{status}</p>
                            )}
                          </div>
                        </div>
                        <Button
                          onClick={() => handleInviteUser(user.user_id)}
                          disabled={!roomHasInviteSlot || status === "Enviando..." || wasInvited}
                          variant="outline"
                          className="w-full text-xs py-1.5 h-8 font-bold"
                        >
                          {status === "Enviando..." ? "Enviando" : wasInvited ? "Convidado" : "Convidar"}
                        </Button>
                      </div>
                    );
                  })
                )}
              </div>
            </section>
          )}
        </div>

        {isFinished && (
          <section className="mt-6 rounded-xl border border-red-500/20 bg-red-500/10 p-4 text-base text-red-750 dark:text-red-100 animate-pulse">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <span className="font-semibold">Partida finalizada. A sala continua aberta para revanche.</span>
              <span className="text-sm text-red-650 dark:text-red-350 font-bold bg-red-50 dark:bg-red-950/40 border border-red-500/10 px-3 py-1.5 rounded-full">
                Dono: {creatorRequestedRematch ? "revanche pedida" : "aguardando"} • Oponente: {opponentRequestedRematch ? "revanche pedida" : "aguardando"}
              </span>
            </div>
          </section>
        )}

        <footer className="mt-8 flex flex-col sm:flex-row gap-3 justify-end border-t border-neutral-200 dark:border-white/5 pt-6">
          <Button onClick={handleLeaveRoom} variant="outline" className="sm:order-1 sm:w-auto px-6 h-10 text-sm font-bold">
            Sair da Sala
          </Button>

          {isPlaying && (
            <Button
              onClick={() => navigate(`/jogar/${room.id}`)}
              className="sm:order-2 sm:w-auto px-6 animate-pulse"
            >
              Voltar ao Jogo
            </Button>
          )}

          {isFinished && (isCreator || isOpponent) && (
            <Button
              onClick={handleRequestRematch}
              disabled={hasRequestedRematch}
              variant={hasRequestedRematch ? "outline" : "solid"}
              className="sm:order-2 sm:w-auto px-6 animate-pulse"
            >
              {hasRequestedRematch ? "Revanche solicitada" : "Pedir Revanche"}
            </Button>
          )}

          {isFinished && isCreator && (
            <Button
              onClick={handleCloseRoom}
              variant="outline"
              className="sm:order-3 sm:w-auto px-6"
            >
              Encerrar Sala
            </Button>
          )}

          {!isFinished && !isPlaying && (isCreator || isOpponent) && (
            <Button
              onClick={handleToggleReady}
              disabled={isCreator ? creatorDisconnected : opponentDisconnected}
              variant={isCreator ? (creatorReady ? "outline" : "solid") : (opponentReady ? "outline" : "solid")}
              className="sm:order-2 sm:w-auto px-6"
            >
              {isCreator ? (creatorReady ? "Não estou pronto" : "Estou Pronto") : (opponentReady ? "Não estou pronto" : "Estou Pronto")}
            </Button>
          )}

          {!isFinished && !isPlaying && isCreator && (
            <Button
              onClick={handleStartMatch}
              disabled={!canStartMatch}
              className="sm:order-3 sm:w-auto px-6"
            >
              Começar Jogo
            </Button>
          )}
        </footer>

      </div>
    </main>
  );
}
