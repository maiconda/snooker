import { useEffect, useRef, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import { getRoom, inviteUser, leaveRoom } from "../lobby/lobbyApi";
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
  const chatEndRef = useRef<HTMLDivElement>(null);

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
    const wsUrl = `${protocol}//${window.location.host}/api/v1/rooms/${activeRoomId}/ws?token=${token}`;
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

  // Rolar chat para o final
  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: "smooth" });
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
    <main className="min-h-screen bg-neutral-950 p-6 text-white">
      <div className="mx-auto flex min-h-[calc(100vh-3rem)] max-w-5xl flex-col justify-center">
        
        {/* Cabeçalho da Sala */}
        <header className="mb-8 flex flex-col justify-between gap-4 border-b border-white/10 pb-6 md:flex-row md:items-end">
          <div>
            <span className="text-xs uppercase tracking-[0.2em] text-emerald-500 font-medium">
              {room.is_private ? "Mesa Privada" : "Mesa Pública"}
            </span>
            <h1 className="mt-1 text-3xl font-bold tracking-tight">Sala de Espera</h1>
            <p className="text-sm text-neutral-500 mt-1">
              ID da partida: {room.id}{isSpectator ? " • Assistindo" : ""}
            </p>
          </div>
          {room.code && (
            <div className="rounded-lg border border-white/10 bg-white/5 p-4 backdrop-blur-sm md:text-right">
              <span className="text-xs text-neutral-500 uppercase tracking-wider block">Código para Amigos</span>
              <strong className="text-2xl font-mono text-emerald-400 tracking-widest">{room.code}</strong>
            </div>
          )}
        </header>

        {/* Grade Principal */}
        <div className="grid gap-6 md:grid-cols-3">
          
          {/* Jogadores (2 colunas) */}
          <section className="grid gap-6 sm:grid-cols-2 md:col-span-2">
            
            {/* Criador */}
            <div className="relative overflow-hidden rounded-xl border border-white/10 bg-white/5 p-6 backdrop-blur-md flex flex-col justify-between min-h-[220px]">
              <div className="flex items-center gap-4">
                {creatorProfile?.photo_url ? (
                  <img
                    src={creatorProfile.photo_url}
                    alt={creatorProfile.nickname}
                    className="h-12 w-12 rounded-full object-cover border border-white/10"
                  />
                ) : (
                  <div className="h-12 w-12 rounded-full bg-gradient-to-tr from-emerald-500 to-teal-500 flex items-center justify-center font-bold text-lg text-white">
                    {creatorProfile?.nickname?.substring(0, 2).toUpperCase() ?? "CR"}
                  </div>
                )}
                <div>
                  <h3 className="font-semibold text-lg">{creatorProfile?.nickname ?? "Carregando..."}</h3>
                  <span className="text-xs text-neutral-500">Dono da Mesa • Nível {Math.floor((creatorProfile?.xp ?? 0) / 100) + 1}</span>
                </div>
              </div>
              
              <div className="mt-8 flex items-center justify-between">
                <span className="text-xs text-neutral-500 uppercase tracking-wider">Estado</span>
                <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold ${
                  creatorDisconnected
                    ? "bg-sky-500/10 text-sky-300"
                    : creatorReady
                      ? "bg-emerald-500/10 text-emerald-400"
                      : "bg-amber-500/10 text-amber-400"
                }`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${
                    creatorDisconnected ? "bg-sky-300" : creatorReady ? "bg-emerald-400" : "bg-amber-400"
                  }`} />
                  {creatorDisconnected ? "Reconectando" : creatorReady ? "Pronto" : "Aguardando"}
                </span>
              </div>
            </div>

            {/* Oponente */}
            <div className="relative overflow-hidden rounded-xl border border-white/10 bg-white/5 p-6 backdrop-blur-md flex flex-col justify-between min-h-[220px]">
              {opponentProfile ? (
                <>
                  <div className="flex items-center gap-4">
                    {opponentProfile?.photo_url ? (
                      <img
                        src={opponentProfile.photo_url}
                        alt={opponentProfile.nickname}
                        className="h-12 w-12 rounded-full object-cover border border-white/10"
                      />
                    ) : (
                      <div className="h-12 w-12 rounded-full bg-gradient-to-tr from-sky-500 to-indigo-500 flex items-center justify-center font-bold text-lg text-white">
                        {opponentProfile?.nickname?.substring(0, 2).toUpperCase() ?? "OP"}
                      </div>
                    )}
                    <div>
                      <h3 className="font-semibold text-lg">{opponentProfile?.nickname}</h3>
                      <span className="text-xs text-neutral-500">Oponente • Nível {Math.floor((opponentProfile?.xp ?? 0) / 100) + 1}</span>
                    </div>
                  </div>
                  
                  <div className="mt-8 flex items-center justify-between">
                    <span className="text-xs text-neutral-500 uppercase tracking-wider">Estado</span>
                    <span className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold ${
                      opponentDisconnected
                        ? "bg-sky-500/10 text-sky-300"
                        : opponentReady
                          ? "bg-emerald-500/10 text-emerald-400"
                          : "bg-amber-500/10 text-amber-400"
                    }`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${
                        opponentDisconnected ? "bg-sky-300" : opponentReady ? "bg-emerald-400" : "bg-amber-400"
                      }`} />
                      {opponentDisconnected ? "Reconectando" : opponentReady ? "Pronto" : "Aguardando"}
                    </span>
                  </div>
                </>
              ) : (
                <div className="flex flex-col items-center justify-center h-full text-center py-6">
                  <div className="h-10 w-10 rounded-full border border-dashed border-white/20 flex items-center justify-center mb-3">
                    <span className="text-neutral-600 text-lg animate-pulse">+</span>
                  </div>
                  <h4 className="text-sm font-medium text-neutral-400">Aguardando Oponente</h4>
                  <p className="text-xs text-neutral-600 mt-1 max-w-[200px]">
                    {room.is_private ? "Compartilhe o código para jogar." : "Partida pública aberta."}
                  </p>
                </div>
              )}
            </div>

          </section>

          {/* Chat em tempo real (1 coluna) */}
          <section className="rounded-xl border border-white/10 bg-white/5 backdrop-blur-md flex flex-col h-[380px] md:h-[450px]">
            <header className="px-4 py-3 border-b border-white/10 flex items-center justify-between">
              <span className="text-xs uppercase tracking-wider text-neutral-400 font-semibold">Mensagens</span>
              <span className="text-[10px] bg-white/10 px-2 py-0.5 rounded text-neutral-400">{messages.length}</span>
            </header>

            {/* Lista de Mensagens */}
            <div className="flex-1 overflow-y-auto p-4 space-y-3">
              {messages.length === 0 ? (
                <div className="h-full flex items-center justify-center text-center text-xs text-neutral-600">
                  Diga olá no chat da partida!
                </div>
              ) : (
                messages.map((msg) => (
                  <div key={msg.messageId} className={`flex flex-col ${msg.senderId === session?.userId ? "items-end" : "items-start"}`}>
                    <span className="text-[10px] text-neutral-500 mb-0.5 px-1">{msg.senderName}</span>
                    <div className={`px-3 py-1.5 rounded-lg text-sm max-w-[85%] break-words ${
                      msg.senderId === session?.userId ? "bg-emerald-600 text-white rounded-tr-none" : "bg-white/10 text-neutral-200 rounded-tl-none"
                    }`}>
                      {msg.text}
                    </div>
                  </div>
                ))
              )}
              <div ref={chatEndRef} />
            </div>

            {/* Input do Chat */}
            <form onSubmit={handleSendChat} className="p-3 border-t border-white/10 flex gap-2">
              <input
                type="text"
                value={messageText}
                onChange={(e) => setMessageText(e.target.value)}
                disabled={isSpectator}
                placeholder={isSpectator ? "Espectadores apenas assistem ao chat" : "Escreva uma mensagem..."}
                className="flex-1 rounded-lg bg-neutral-900 border border-white/10 px-3 py-1.5 text-sm placeholder-neutral-500 focus:outline-none focus:border-emerald-500 text-white"
              />
              <button
                type="submit"
                disabled={isSpectator || !messageText.trim()}
                className="rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:bg-neutral-800 disabled:text-neutral-600 px-3 text-sm font-semibold transition"
              >
                Enviar
              </button>
            </form>
          </section>

        </div>

        {/* Rodapé de Ações */}
        <div className="mt-6 grid gap-6 md:grid-cols-2">
          <section className="rounded-xl border border-white/10 bg-white/5 p-5 backdrop-blur-md">
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-semibold uppercase tracking-wider text-neutral-300">Espectadores</h2>
              <span className="rounded-full bg-white/10 px-2 py-0.5 text-xs text-neutral-400">{spectators.length}</span>
            </div>
            <div className="mt-4 space-y-2">
              {spectators.length === 0 ? (
                <p className="text-xs text-neutral-600">Ninguem assistindo no momento.</p>
              ) : (
                spectators.map((spectator) => (
                  <div key={spectator.user_id} className="flex items-center gap-3 rounded-lg bg-white/5 px-3 py-2">
                    {profilesById[spectator.user_id]?.photo_url ? (
                      <img
                        src={profilesById[spectator.user_id]?.photo_url}
                        alt={profilesById[spectator.user_id]?.nickname ?? "Espectador"}
                        className="h-8 w-8 rounded-full object-cover"
                      />
                    ) : (
                      <div className="flex h-8 w-8 items-center justify-center rounded-full bg-neutral-800 text-xs font-semibold text-neutral-300">
                        {(profilesById[spectator.user_id]?.nickname ?? "ES").substring(0, 2).toUpperCase()}
                      </div>
                    )}
                    <span className="text-sm text-neutral-200">{profilesById[spectator.user_id]?.nickname ?? "Carregando..."}</span>
                  </div>
                ))
              )}
            </div>
          </section>

          {isCreator && (
            <section className="rounded-xl border border-white/10 bg-white/5 p-5 backdrop-blur-md">
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-semibold uppercase tracking-wider text-neutral-300">Jogadores online</h2>
                <span className="rounded-full bg-white/10 px-2 py-0.5 text-xs text-neutral-400">{onlineInviteCandidates.length}</span>
              </div>
              <div className="mt-4 space-y-2">
                {onlineInviteCandidates.length === 0 ? (
                  <p className="text-xs text-neutral-600">Nenhum jogador online disponivel para convite.</p>
                ) : (
                  onlineInviteCandidates.map((user) => {
                    const status = inviteStatuses[user.user_id];
                    const wasInvited = Boolean(status && status !== "Enviando..." && !status.startsWith("Falha") && !status.startsWith("Usuario"));
                    return (
                      <div key={user.user_id} className="flex items-center justify-between gap-3 rounded-lg bg-white/5 px-3 py-2">
                        <div className="flex min-w-0 items-center gap-3">
                          {profilesById[user.user_id]?.photo_url ? (
                            <img
                              src={profilesById[user.user_id]?.photo_url}
                              alt={profilesById[user.user_id]?.nickname ?? "Jogador"}
                              className="h-8 w-8 rounded-full object-cover"
                            />
                          ) : (
                            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-neutral-800 text-xs font-semibold text-neutral-300">
                              {(profilesById[user.user_id]?.nickname ?? "ON").substring(0, 2).toUpperCase()}
                            </div>
                          )}
                          <div className="min-w-0">
                            <p className="truncate text-sm text-neutral-200">{profilesById[user.user_id]?.nickname ?? "Carregando..."}</p>
                            {status && !wasInvited && status !== "Enviando..." && (
                              <p className="truncate text-[10px] text-red-300">{status}</p>
                            )}
                          </div>
                        </div>
                        <Button
                          onClick={() => handleInviteUser(user.user_id)}
                          disabled={!roomHasInviteSlot || status === "Enviando..." || wasInvited}
                          variant="outline"
                          className="px-3 py-1.5 text-xs"
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
          <section className="mt-6 rounded-xl border border-emerald-500/20 bg-emerald-500/10 p-4 text-sm text-emerald-100">
            <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
              <span>Partida finalizada. A sala continua aberta para revanche.</span>
              <span className="text-xs text-emerald-200">
                Dono: {creatorRequestedRematch ? "revanche solicitada" : "aguardando"} • Oponente: {opponentRequestedRematch ? "revanche solicitada" : "aguardando"}
              </span>
            </div>
          </section>
        )}

        <footer className="mt-8 flex flex-col sm:flex-row gap-3 justify-end border-t border-white/10 pt-6">
          <Button onClick={handleLeaveRoom} variant="outline" className="sm:order-1">
            Sair da Sala
          </Button>

          {isPlaying && (
            <Button
              onClick={() => navigate(`/jogar/${room.id}`)}
              className="sm:order-2 bg-emerald-600 hover:bg-emerald-500 text-white"
            >
              Voltar ao Jogo
            </Button>
          )}

          {isFinished && (isCreator || isOpponent) && (
            <Button
              onClick={handleRequestRematch}
              disabled={hasRequestedRematch}
              variant={hasRequestedRematch ? "outline" : "solid"}
              className="sm:order-2"
            >
              {hasRequestedRematch ? "Revanche solicitada" : "Pedir Revanche"}
            </Button>
          )}

          {isFinished && isCreator && (
            <Button
              onClick={handleCloseRoom}
              variant="outline"
              className="sm:order-3"
            >
              Encerrar Sala
            </Button>
          )}

          {!isFinished && !isPlaying && (isCreator || isOpponent) && (
            <Button
              onClick={handleToggleReady}
              disabled={isCreator ? creatorDisconnected : opponentDisconnected}
              variant={isCreator ? (creatorReady ? "outline" : "solid") : (opponentReady ? "outline" : "solid")}
              className="sm:order-2"
            >
              {isCreator ? (creatorReady ? "Não estou pronto" : "Estou Pronto") : (opponentReady ? "Não estou pronto" : "Estou Pronto")}
            </Button>
          )}

          {!isFinished && !isPlaying && isCreator && (
            <Button
              onClick={handleStartMatch}
              disabled={!canStartMatch}
              className="sm:order-3 bg-emerald-600 hover:bg-emerald-500 text-white"
            >
              Começar Jogo
            </Button>
          )}
        </footer>

      </div>
    </main>
  );
}
