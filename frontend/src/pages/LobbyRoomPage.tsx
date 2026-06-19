import { useEffect, useRef, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import { getRoom, joinRoom } from "../lobby/lobbyApi";
import type { Room, WSEvent } from "../lobby/types";
import { getMyProfile, getPublicProfile } from "../profile/profileApi";
import type { Profile } from "../profile/types";
import { navigate } from "../lib/router";

type ChatMessage = {
  senderId: string;
  senderName: string;
  text: string;
  timestamp: number;
};

export function LobbyRoomPage({ roomId }: { roomId: string }) {
  const { session } = useAuth();
  const [room, setRoom] = useState<Room | null>(null);
  const [creatorProfile, setCreatorProfile] = useState<Profile | null>(null);
  const [opponentProfile, setOpponentProfile] = useState<Profile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Estados de prontidão locais
  const [creatorReady, setCreatorReady] = useState(false);
  const [opponentReady, setOpponentReady] = useState(false);

  // Chat
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [messageText, setMessageText] = useState("");
  const chatEndRef = useRef<HTMLDivElement>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const profilesCache = useRef<Record<string, Profile>>({});
  const roomRef = useRef<Room | null>(null);
  roomRef.current = room;

  // Carregar dados iniciais da sala e perfis
  useEffect(() => {
    let active = true;
    const token = session?.accessToken;
    if (!token) return;

    async function loadData() {
      const activeToken = session?.accessToken;
      if (!activeToken || !roomId) return;
      try {
        setLoading(true);
        // Obter detalhes da sala
        const roomData = await getRoom(activeToken, roomId);
        if (!active) return;
        setRoom(roomData);

        // Carregar perfil do criador
        const creator = await getPublicProfile(activeToken, roomData.creator_id);
        if (!active) return;
        setCreatorProfile(creator);
        profilesCache.current[roomData.creator_id] = creator;

        // Carregar perfil do oponente se houver
        if (roomData.opponent_id) {
          const opponent = await getPublicProfile(activeToken, roomData.opponent_id);
          if (!active) return;
          setOpponentProfile(opponent);
          profilesCache.current[roomData.opponent_id] = opponent;
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

  // Conectar WebSocket
  useEffect(() => {
    const token = session?.accessToken;
    const activeRoomId = room?.id;
    if (!token || !activeRoomId) return;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/api/v1/rooms/${activeRoomId}/ws?token=${token}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      console.log("WebSocket conectado ao Lobby da Sala:", activeRoomId);
    };

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

        switch (wsMsg.type) {
          case "player_joined":
            // Recarrega a sala para obter novo oponente se necessário
            const updatedRoom = await getRoom(token, currentRoom.id);
            setRoom(updatedRoom);
            if (updatedRoom.opponent_id && !profilesCache.current[updatedRoom.opponent_id]) {
              const opp = await getPublicProfile(token, updatedRoom.opponent_id);
              setOpponentProfile(opp);
              profilesCache.current[updatedRoom.opponent_id] = opp;
            }
            break;

          case "player_left":
            if (senderId === currentRoom.creator_id) {
              setError("O criador saiu da sala. Esta sala não é mais válida.");
            } else {
              setOpponentProfile(null);
              setOpponentReady(false);
              setRoom((prev) => prev ? { ...prev, opponent_id: undefined } : null);
            }
            break;

          case "chat_message":
            const payload = JSON.parse(JSON.stringify(wsMsg.payload)) as { text: string };
            setMessages((prev) => [
              ...prev,
              {
                senderId,
                senderName,
                text: payload.text,
                timestamp: Date.now()
              }
            ]);
            break;

          case "player_ready":
            const readyPayload = JSON.parse(JSON.stringify(wsMsg.payload)) as { ready: boolean };
            if (senderId === currentRoom.creator_id) {
              setCreatorReady(readyPayload.ready);
            } else {
              setOpponentReady(readyPayload.ready);
            }
            break;

          case "match_start":
            // Redireciona para o Game Core
            navigate(`/jogar/${currentRoom.id}`);
            break;

          case "room_closed":
            setError("O dono da sala se desconectou. A sala foi encerrada.");
            break;

          default:
            break;
        }
      } catch (err) {
        console.error("Erro ao processar mensagem WebSocket:", err);
      }
    };

    ws.onclose = () => {
      console.log("WebSocket desconectado");
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
    if (!messageText.trim() || !wsRef.current) return;

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
    if (!wsRef.current) return;
    const eventMsg = {
      type: "match_start"
    };
    wsRef.current.send(JSON.stringify(eventMsg));
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
  const bothReady = creatorReady && opponentReady;

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
            <p className="text-sm text-neutral-500 mt-1">ID da partida: {room.id}</p>
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
                  creatorReady ? "bg-emerald-500/10 text-emerald-400" : "bg-amber-500/10 text-amber-400"
                }`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${creatorReady ? "bg-emerald-400" : "bg-amber-400"}`} />
                  {creatorReady ? "Pronto" : "Aguardando"}
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
                      opponentReady ? "bg-emerald-500/10 text-emerald-400" : "bg-amber-500/10 text-amber-400"
                    }`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${opponentReady ? "bg-emerald-400" : "bg-amber-400"}`} />
                      {opponentReady ? "Pronto" : "Aguardando"}
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
                messages.map((msg, i) => (
                  <div key={i} className={`flex flex-col ${msg.senderId === session?.userId ? "items-end" : "items-start"}`}>
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
                placeholder="Escreva uma mensagem..."
                className="flex-1 rounded-lg bg-neutral-900 border border-white/10 px-3 py-1.5 text-sm placeholder-neutral-500 focus:outline-none focus:border-emerald-500 text-white"
              />
              <button
                type="submit"
                disabled={!messageText.trim()}
                className="rounded-lg bg-emerald-600 hover:bg-emerald-500 disabled:bg-neutral-800 disabled:text-neutral-600 px-3 text-sm font-semibold transition"
              >
                Enviar
              </button>
            </form>
          </section>

        </div>

        {/* Rodapé de Ações */}
        <footer className="mt-8 flex flex-col sm:flex-row gap-3 justify-end border-t border-white/10 pt-6">
          <Button onClick={() => navigate("/")} variant="outline" className="sm:order-1">
            Sair da Sala
          </Button>

          {(isCreator || isOpponent) && (
            <Button
              onClick={handleToggleReady}
              variant={isCreator ? (creatorReady ? "outline" : "solid") : (opponentReady ? "outline" : "solid")}
              className="sm:order-2"
            >
              {isCreator ? (creatorReady ? "Não estou pronto" : "Estou Pronto") : (opponentReady ? "Não estou pronto" : "Estou Pronto")}
            </Button>
          )}

          {isCreator && (
            <Button
              onClick={handleStartMatch}
              disabled={!bothReady}
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
