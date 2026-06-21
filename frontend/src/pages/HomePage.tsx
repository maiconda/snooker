import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import { navigate } from "../lib/router";
import { createRoom, listPublicRooms, joinRoom } from "../lobby/lobbyApi";
import type { Room, WSEvent } from "../lobby/types";
import { getMyProfile, getPublicProfile } from "../profile/profileApi";
import type { Profile } from "../profile/types";

export function HomePage() {
  const { logout, session } = useAuth();
  const [loading, setLoading] = useState(false);
  const [myProfile, setMyProfile] = useState<Profile | null>(null);

  // Estados do Lobby
  const [rooms, setRooms] = useState<Room[]>([]);
  const [roomCreatorNames, setRoomCreatorNames] = useState<Record<string, string>>({});
  const [isPrivate, setIsPrivate] = useState(false);
  const [roomCode, setRoomCode] = useState("");
  const [lobbyError, setLobbyError] = useState<string | null>(null);

  // Carregar perfil do jogador conectado e salas públicas
  useEffect(() => {
    let active = true;
    const token = session?.accessToken;
    if (!token) return;

    // Obter perfil próprio
    getMyProfile(token)
      .then((profile) => {
        if (active) setMyProfile(profile);
      })
      .catch((err) => console.error("Erro ao buscar perfil:", err));

    // Obter salas públicas iniciais
    fetchRooms(token);

    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    const connectRoomsWS = () => {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/rooms/public/ws?token=${token}`);

      ws.onmessage = (event) => {
        try {
          const wsMsg = JSON.parse(event.data) as WSEvent<Room[]>;
          if (!active || wsMsg.type !== "public_rooms_snapshot" || !Array.isArray(wsMsg.payload)) return;
          setRooms(wsMsg.payload);
        } catch (err) {
          console.error("Erro ao processar salas em tempo real:", err);
        }
      };

      ws.onclose = () => {
        if (!active) return;
        reconnectTimer = setTimeout(connectRoomsWS, 2000);
      };
    };

    connectRoomsWS();

    return () => {
      active = false;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, [session?.accessToken]);

  useEffect(() => {
    let active = true;
    const token = session?.accessToken;
    if (!token) return;

    rooms.forEach(async (room) => {
      if (roomCreatorNames[room.creator_id]) return;

      try {
        const creator = await getPublicProfile(token, room.creator_id);
        if (!active) return;

        setRoomCreatorNames((prev) => ({
          ...prev,
          [room.creator_id]: creator.nickname
        }));
      } catch (err) {
        console.error("Erro ao buscar apelido do criador:", err);
      }
    });

    return () => {
      active = false;
    };
  }, [rooms, roomCreatorNames, session?.accessToken]);

  const fetchRooms = async (token: string) => {
    try {
      const publicRooms = await listPublicRooms(token);
      setRooms(publicRooms);
    } catch (err) {
      console.error("Erro ao carregar salas públicas:", err);
    }
  };

  const handleLogout = async () => {
    setLoading(true);
    try {
      await logout();
    } catch (error) {
      console.error("Erro ao fazer logout:", error);
    } finally {
      setLoading(false);
    }
  };

  // Criar nova sala
  const handleCreateRoom = async () => {
    const token = session?.accessToken;
    if (!token) return;
    setLobbyError(null);

    try {
      const newRoom = await createRoom(token, isPrivate);
      navigate(`/sala/${newRoom.id}`);
    } catch (err) {
      setLobbyError(err instanceof Error ? err.message : "Erro ao criar sala.");
    }
  };

  // Entrar por código
  const handleJoinByCode = async (e: React.FormEvent) => {
    e.preventDefault();
    const token = session?.accessToken;
    if (!token || !roomCode.trim()) return;
    setLobbyError(null);

    try {
      const joined = await joinRoom(token, roomCode.trim().toUpperCase());
      navigate(`/sala/${joined.id}`);
    } catch (err) {
      setLobbyError(err instanceof Error ? err.message : "Código de sala inválido ou expirado.");
    }
  };

  // Entrar em sala pública
  const handleJoinPublicRoom = async (roomId: string) => {
    const token = session?.accessToken;
    if (!token) return;
    setLobbyError(null);

    try {
      const joined = await joinRoom(token, roomId);
      navigate(`/sala/${joined.id}`);
    } catch (err) {
      setLobbyError(err instanceof Error ? err.message : "Falha ao entrar na sala pública.");
    }
  };

  const handleWatchPublicRoom = (roomId: string) => {
    navigate(`/sala/${roomId}`);
  };

  return (
    <main className="min-h-screen bg-neutral-950 p-6 text-white animate-fade-in" aria-label="Pagina inicial">
      <section className="mx-auto flex min-h-screen w-full max-w-4xl flex-col justify-center space-y-6">
        
        {/* Painel do Perfil (Layout Original Adaptado) */}
        <div className="border border-white/10 bg-white p-6 text-black rounded-lg transition hover:shadow-lg">
          <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
            <div>
              <p className="text-xs uppercase tracking-[0.18em] text-neutral-500">Snooker Multiplayer</p>
              <h1 className="mt-2 text-3xl font-semibold tracking-normal">
                Olá, {myProfile?.nickname ?? "Jogador"}
              </h1>
              <p className="mt-1 text-sm text-neutral-600">
                {session?.email ?? "Jogador conectado"} • Nível {myProfile ? Math.floor(myProfile.xp / 100) + 1 : 1}
              </p>
            </div>
            
            <div className="flex gap-2">
              <Button onClick={() => navigate("/perfil")}>Editar perfil</Button>
              <Button onClick={handleLogout} disabled={loading} variant="outline">
                {loading ? "Saindo..." : "Logout"}
              </Button>
            </div>
          </div>
        </div>

        {/* Mensagem de Erro do Lobby */}
        {lobbyError && (
          <div className="bg-red-500/10 border border-red-500/20 text-red-400 p-4 rounded-lg text-sm">
            {lobbyError}
          </div>
        )}

        {/* Grade de Lobbies */}
        <div className="grid gap-6 md:grid-cols-2">
          
          {/* Seção Criar e Entrar por Código */}
          <div className="border border-white/10 bg-zinc-900/50 p-6 rounded-lg space-y-8">
            
            {/* Criar Sala */}
            <div>
              <h2 className="text-lg font-semibold mb-3">Abrir Nova Mesa</h2>
              <p className="text-xs text-neutral-400 mb-4">Inicie uma nova mesa para jogar contra amigos ou contra oponentes públicos.</p>
              
              <div className="flex items-center gap-2 mb-4">
                <input
                  type="checkbox"
                  id="private-room"
                  checked={isPrivate}
                  onChange={(e) => setIsPrivate(e.target.checked)}
                  className="h-4 w-4 rounded border-white/10 bg-neutral-900 text-emerald-600 focus:ring-emerald-500 focus:ring-offset-neutral-950"
                />
                <label htmlFor="private-room" className="text-sm select-none">
                  Mesa Privada (acesso apenas com código de convite)
                </label>
              </div>

              <Button onClick={handleCreateRoom} variant="outline" className="w-full">
                Criar Mesa
              </Button>
            </div>

            <hr className="border-white/10" />

            {/* Entrar por Código */}
            <div>
              <h2 className="text-lg font-semibold mb-3">Entrar por Código</h2>
              <p className="text-xs text-neutral-400 mb-4">Digite o código de 6 caracteres para entrar em uma mesa particular criada.</p>
              
              <form onSubmit={handleJoinByCode} className="flex gap-2">
                <input
                  type="text"
                  maxLength={6}
                  value={roomCode}
                  onChange={(e) => setRoomCode(e.target.value.toUpperCase())}
                  placeholder="CÓDIGO"
                  className="w-32 text-center font-mono font-bold tracking-widest rounded-lg bg-neutral-900 border border-white/10 px-3 py-2 text-white uppercase focus:outline-none focus:border-emerald-500"
                />
                <Button type="submit" disabled={!roomCode.trim()} variant="outline" className="flex-1">
                  Entrar
                </Button>
              </form>
            </div>

          </div>

          {/* Lista de Mesas Públicas */}
          <div className="border border-white/10 bg-zinc-900/50 p-6 rounded-lg flex flex-col h-[380px]">
            <div className="flex items-center justify-between mb-4">
              <div>
                <h2 className="text-lg font-semibold">Mesas Públicas</h2>
                <p className="text-xs text-neutral-400">Jogue com outras pessoas da comunidade.</p>
              </div>
              <button
                onClick={() => {
                  const token = session?.accessToken;
                  if (token) fetchRooms(token);
                }}
                className="text-xs text-emerald-400 hover:text-emerald-300 font-medium"
              >
                Atualizar
              </button>
            </div>

            <div className="flex-1 overflow-y-auto space-y-3 pr-2">
              {rooms.length === 0 ? (
                <div className="h-full flex flex-col items-center justify-center text-center text-xs text-neutral-600">
                  <p>Nenhuma mesa pública aberta no momento.</p>
                  <p className="mt-1">Crie a primeira para começar!</p>
                </div>
              ) : (
                rooms.map((room) => {
                  const creatorName = roomCreatorNames[room.creator_id] ?? "Carregando...";
                  const creatorDisconnected = Boolean(room.creator_disconnected_at);
                  const opponentDisconnected = Boolean(room.opponent_disconnected_at);
                  const hasReconnectWindow = creatorDisconnected || opponentDisconnected;
                  const canPlay = !hasReconnectWindow && room.status === "waiting" && !room.opponent_id;
                  const canReconnect = (creatorDisconnected && session?.userId === room.creator_id)
                    || (opponentDisconnected && session?.userId === room.opponent_id);
                  let roomStatus = "Mesa completa";
                  if (creatorDisconnected && opponentDisconnected) {
                    roomStatus = "Jogadores reconectando";
                  } else if (creatorDisconnected) {
                    roomStatus = "Dono reconectando";
                  } else if (opponentDisconnected) {
                    roomStatus = "Oponente reconectando";
                  } else if (room.status === "playing") {
                    roomStatus = "Em partida";
                  } else if (room.status === "finished") {
                    roomStatus = "Aguardando revanche";
                  } else if (canPlay) {
                    roomStatus = "Aguardando jogador";
                  }
                  return (
                    <div key={room.id} className="flex items-center justify-between gap-3 p-4 rounded-lg bg-white/5 border border-white/5 hover:border-white/10 transition">
                      <div>
                        <span className="text-xs text-neutral-400 uppercase tracking-wider block">Mesa de</span>
                        <strong className="text-sm font-semibold">{creatorName}</strong>
                        <span className="mt-1 block text-[10px] uppercase tracking-wider text-neutral-500">{roomStatus}</span>
                      </div>
                      <div className="flex shrink-0 gap-2">
                        {canReconnect ? (
                          <Button onClick={() => handleWatchPublicRoom(room.id)} variant="outline" className="px-3 py-1.5 text-xs">
                            Reconectar
                          </Button>
                        ) : (
                          <Button
                            onClick={() => handleJoinPublicRoom(room.id)}
                            disabled={!canPlay}
                            variant="outline"
                            className="px-3 py-1.5 text-xs"
                          >
                            Jogar
                          </Button>
                        )}
                        {!canReconnect && (
                          <Button onClick={() => handleWatchPublicRoom(room.id)} variant="outline" className="px-3 py-1.5 text-xs">
                            Assistir
                          </Button>
                        )}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>

        </div>

      </section>
    </main>
  );
}
