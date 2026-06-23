import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import { useTheme } from "../components/ThemeContext";
import { navigate } from "../lib/router";
import { createRoom, listPublicRooms, joinRoom } from "../lobby/lobbyApi";
import type { Room, WSEvent } from "../lobby/types";
import { getMyProfile, getPublicProfile } from "../profile/profileApi";
import type { Profile } from "../profile/types";

export function HomePage() {
  const { logout, session } = useAuth();
   const { theme } = useTheme();
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
          setRooms((prev) => (sameRoomList(prev, wsMsg.payload ?? []) ? prev : wsMsg.payload ?? []));
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
      setRooms((prev) => (sameRoomList(prev, publicRooms) ? prev : publicRooms));
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

  const totalXP = myProfile?.xp ?? 0;
  const level = Math.floor(totalXP / 100) + 1;
  const levelProgress = totalXP % 100;

  return (
    <main className="relative min-h-screen lg:h-screen lg:overflow-hidden bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-red-100/30 via-zinc-50 to-zinc-50 dark:from-red-950/20 dark:via-neutral-950 dark:to-neutral-950 p-6 text-neutral-900 dark:text-white transition-colors duration-300 animate-fade-in flex flex-col" aria-label="Pagina inicial">
      <div className="absolute inset-0 bg-[linear-gradient(to_bottom,rgba(0,0,0,0.015)_1px,transparent_1px)] dark:bg-[linear-gradient(to_bottom,rgba(255,255,255,0.005)_1px,transparent_1px)] bg-[size:100%_40px] pointer-events-none" />
      
      {/* Container Principal que Envolve Tudo */}
      <div className="relative mx-auto flex w-full max-w-7xl flex-col flex-1 min-h-0">
        
        {/* Barra de Topo */}
        <header className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-neutral-200 dark:border-white/10 pb-4 mb-6 shrink-0 z-10">
          <div className="flex items-center gap-3">
            <img src="/logo.png" alt="Snooker Club Logo" className="h-10 w-auto object-contain" />
          </div>
          <div className="flex items-center gap-3">
            <Button onClick={handleLogout} disabled={loading} variant="outline" className="!w-auto px-4 border-red-500/20 text-red-500 dark:text-red-400 hover:bg-red-500/10 dark:hover:bg-red-500/10 hover:border-red-500/30">
              {loading ? "Saindo..." : "Logout"}
            </Button>
          </div>
        </header>

        {/* Conteúdo Principal em Grid */}
        <section className="grid w-full gap-6 grid-cols-1 lg:grid-cols-[300px_1fr_360px] items-stretch flex-1 min-h-0 lg:max-h-[680px] overflow-y-auto lg:overflow-visible pb-6 lg:pb-0">
        
        {/* Painel Esquerdo: Jogador */}
        <aside className="h-full min-h-0">
          <div className="flex flex-col items-center text-center p-6 border border-neutral-200 dark:border-white/10 bg-white/80 dark:bg-zinc-900/60 rounded-2xl shadow-xl backdrop-blur-xl transition hover:border-red-500/10 dark:hover:border-red-500/10 h-full justify-between min-h-0">
            <div className="w-full flex flex-col items-center">
              <div className="relative mb-4 shrink-0">
                <div className="h-24 w-24 overflow-hidden border-2 border-red-650/50 dark:border-red-500/40 rounded-2xl bg-neutral-100 dark:bg-neutral-800 shadow-md">
                  {myProfile?.photo_url ? (
                    <img src={myProfile.photo_url} alt="" className="h-full w-full object-cover" />
                  ) : (
                    <div className="flex h-full w-full items-center justify-center text-neutral-400 dark:text-neutral-500">
                      <svg className="h-10 w-10 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 6a3.75 3.75 0 1 1-7.5 0 3.75 3.75 0 0 1 7.5 0ZM4.501 20.118a7.5 7.5 0 0 1 14.998 0A17.933 17.933 0 0 1 12 21.75c-2.676 0-5.216-.584-7.499-1.632Z" />
                      </svg>
                    </div>
                  )}
                </div>
                <span className="absolute -bottom-1 -right-1 flex h-4.5 w-4.5 items-center justify-center rounded-full bg-green-500 border-2 border-white dark:border-neutral-900 shadow-sm" title="Online">
                  <span className="h-2 w-2 rounded-full bg-white animate-pulse" />
                </span>
              </div>
              
              <h2 className="text-2xl font-black tracking-tight text-neutral-900 dark:text-white break-words w-full px-1 shrink-0">
                {myProfile?.nickname ?? "Jogador"}
              </h2>
              <p className="text-sm font-semibold text-neutral-500 dark:text-neutral-400 break-all w-full px-1 mt-1 shrink-0">
                {session?.email ?? "Conectado"}
              </p>

              {myProfile?.bio ? (
                <p className="text-sm leading-relaxed text-neutral-600 dark:text-neutral-350 mt-3 line-clamp-3 break-words max-w-[260px] px-2 shrink-0">
                  {myProfile.bio}
                </p>
              ) : (
                <p className="text-sm italic text-neutral-400 dark:text-neutral-500 mt-3 px-2 shrink-0">
                  Sem biografia definida
                </p>
              )}
            </div>

            {/* Estatísticas Rápidas Integradas (Modo Premium) */}
            <div className="w-full my-4 grid grid-cols-2 gap-3 shrink-0">
              <div className="bg-neutral-100 dark:bg-neutral-950/60 rounded-xl border border-neutral-200 dark:border-white/5 p-3 text-center shadow-sm">
                <span className="text-sm uppercase font-bold text-neutral-500 dark:text-neutral-400 tracking-wider block mb-0.5">Total XP</span>
                <strong className="text-2xl font-black text-red-650 dark:text-red-500">{totalXP}</strong>
              </div>
              <div className="bg-neutral-100 dark:bg-neutral-950/60 rounded-xl border border-neutral-200 dark:border-white/5 p-3 text-center shadow-sm">
                <span className="text-sm uppercase font-bold text-neutral-500 dark:text-neutral-400 tracking-wider block mb-0.5">Nível</span>
                <strong className="text-2xl font-black text-red-650 dark:text-red-500">{level}</strong>
              </div>
            </div>

            {/* Level e XP Progress Bar */}
            <div className="w-full border-t border-neutral-200 dark:border-white/10 pt-3 text-left shrink-0">
              <div className="flex items-center justify-between text-sm font-extrabold text-neutral-800 dark:text-neutral-250 shrink-0">
                <span className="uppercase tracking-wider text-amber-500 dark:text-amber-400 text-sm">Progresso</span>
                <span className="font-mono text-neutral-550 text-sm font-bold">{levelProgress}/100 XP</span>
              </div>
              <div className="mt-2 h-2.5 w-full rounded-full bg-neutral-200 dark:bg-neutral-950 border border-neutral-300 dark:border-white/10 overflow-hidden shrink-0">
                <div className="h-full bg-gradient-to-r from-red-600 to-rose-500 rounded-full shadow-[0_0_10px_rgba(239,68,68,0.6)]" style={{ width: `${levelProgress}%` }} />
              </div>
            </div>

            <Button onClick={() => navigate("/perfil")} variant="outline" className="w-full mt-4 h-10 text-sm font-bold uppercase tracking-wider shrink-0">
              Editar perfil
            </Button>
          </div>
        </aside>

        {/* Painel Central: Modos de Jogo / Play Hero */}
        <div className="h-full min-h-0">
          <div className="border border-neutral-200 dark:border-white/10 bg-white/80 dark:bg-zinc-900/60 p-6 rounded-2xl shadow-xl backdrop-blur-xl flex flex-col justify-between h-full min-h-0">
            {/* Seção Superior: Iniciar Partida */}
            <div className="space-y-4">
              <div>
                <h2 className="text-xl font-extrabold tracking-wide text-neutral-900 dark:text-white mb-1">Iniciar Partida</h2>
                <p className="text-sm text-neutral-500 dark:text-neutral-400">Crie uma nova mesa para jogar online contra oponentes públicos ou privados.</p>
              </div>

              {lobbyError && (
                <div className="bg-red-500/10 border border-red-500/20 text-red-550 dark:text-red-400 px-4 py-2 rounded-xl text-sm animate-fade-in font-semibold shrink-0">
                  {lobbyError}
                </div>
              )}

              <div className="grid gap-4 sm:grid-cols-2">
                {/* Opção Pública */}
                <button 
                  type="button"
                  onClick={() => setIsPrivate(false)}
                  className={`flex flex-col text-left justify-between p-5 border-2 rounded-xl cursor-pointer transition-all duration-200 outline-none focus:ring-2 focus:ring-red-500/20 ${
                    !isPrivate 
                      ? "border-red-600 bg-red-500/[0.03] dark:bg-red-500/[0.02] shadow-lg shadow-red-550/5" 
                      : "border-neutral-200 dark:border-white/5 hover:border-neutral-300 dark:hover:border-white/10 hover:bg-neutral-500/[0.02]"
                  }`}
                >
                  <div className="flex flex-col gap-3">
                    <div className={`p-2 rounded-lg w-fit ${!isPrivate ? "bg-red-500/10 text-red-600 dark:text-red-500" : "bg-neutral-100 dark:bg-neutral-900 text-neutral-400 dark:text-neutral-500"}`}>
                      <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M18.364 5.636l-3.536 3.536m0 5.656l3.536 3.536M9.172 9.172L5.636 5.636m3.536 9.192l-3.536 3.536M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-5 0a4 4 0 11-8 0 4 4 0 018 0z" />
                      </svg>
                    </div>
                    <div>
                      <strong className="text-base font-black text-neutral-900 dark:text-white block">Mesa Pública</strong>
                      <span className="text-sm leading-relaxed text-neutral-550 dark:text-neutral-400 mt-1 block">
                        Qualquer jogador do lobby poderá entrar ou assistir sua partida livremente.
                      </span>
                    </div>
                  </div>
                </button>

                {/* Opção Privada */}
                <button 
                  type="button"
                  onClick={() => setIsPrivate(true)}
                  className={`flex flex-col text-left justify-between p-5 border-2 rounded-xl cursor-pointer transition-all duration-200 outline-none focus:ring-2 focus:ring-red-500/20 ${
                    isPrivate 
                      ? "border-red-600 bg-red-500/[0.03] dark:bg-red-500/[0.02] shadow-lg shadow-red-550/5" 
                      : "border-neutral-200 dark:border-white/5 hover:border-neutral-350 dark:hover:border-white/10 hover:bg-neutral-500/[0.02]"
                  }`}
                >
                  <div className="flex flex-col gap-3">
                    <div className={`p-2 rounded-lg w-fit ${isPrivate ? "bg-red-500/10 text-red-600 dark:text-red-500" : "bg-neutral-100 dark:bg-neutral-900 text-neutral-400 dark:text-neutral-500"}`}>
                      <svg className="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                      </svg>
                    </div>
                    <div>
                      <strong className="text-base font-black text-neutral-900 dark:text-white block">Mesa Privada</strong>
                      <span className="text-sm leading-relaxed text-neutral-550 dark:text-neutral-400 mt-1 block">
                        Apenas oponentes com o código de acesso de 6 dígitos poderão se conectar.
                      </span>
                    </div>
                  </div>
                </button>
              </div>

              <Button onClick={handleCreateRoom} variant="solid" className="w-full h-12 text-sm font-black tracking-widest uppercase shrink-0">
                {isPrivate ? "Criar Mesa Privada" : "Abrir Mesa Pública"}
              </Button>
            </div>

            {/* Separador */}
            <div className="relative my-4 shrink-0">
              <div className="absolute inset-0 flex items-center" aria-hidden="true">
                <div className="w-full border-t border-neutral-200 dark:border-white/5" />
              </div>
              <div className="relative flex justify-center text-xs uppercase">
                <span className="bg-white dark:bg-[#131317] px-2 font-bold text-neutral-450 dark:text-neutral-500 tracking-widest">OU</span>
              </div>
            </div>

            {/* Seção Inferior: Entrar por Código */}
            <div className="space-y-2 shrink-0">
              <div>
                <h3 className="text-base font-bold text-neutral-900 dark:text-white">Entrar por Código</h3>
                <p className="text-sm text-neutral-500 dark:text-neutral-500">Insira o código de 6 caracteres fornecido pelo seu amigo.</p>
              </div>
              
              <form onSubmit={handleJoinByCode} className="flex gap-3">
                <input
                  type="text"
                  maxLength={6}
                  value={roomCode}
                  onChange={(e) => setRoomCode(e.target.value.toUpperCase())}
                  placeholder="CÓDIGO"
                  className="w-32 text-center font-mono font-black text-lg tracking-[0.2em] rounded-lg bg-neutral-50 dark:bg-neutral-950/50 border border-neutral-300 dark:border-white/10 px-3 py-2 text-red-650 dark:text-red-500 placeholder:text-neutral-300 dark:placeholder:text-neutral-800 focus:outline-none focus:border-red-500/60 focus:ring-2 focus:ring-red-500/10 transition-all uppercase"
                />
                <Button type="submit" disabled={!roomCode.trim()} variant="outline" className="flex-1 h-10 text-sm font-bold">
                  Conectar na Mesa
                </Button>
              </form>
            </div>
          </div>
        </div>

        {/* Coluna Direita: Mesas Públicas / Lobby Browser */}
        <aside className="border border-neutral-200 dark:border-white/10 bg-white/80 dark:bg-zinc-900/60 p-5 rounded-2xl shadow-xl backdrop-blur-xl flex flex-col h-full min-h-0">
          <div className="flex items-center justify-between mb-4 pb-3 border-b border-neutral-200 dark:border-white/10 shrink-0">
              <div>
                <h2 className="text-xl font-bold tracking-wide text-neutral-900 dark:text-white flex items-center gap-2">
                  Mesas Ativas
                  <span className="inline-flex items-center justify-center px-2.5 py-0.5 text-sm font-bold rounded-full bg-neutral-100 dark:bg-white/5 text-neutral-550 dark:text-neutral-400">
                    {rooms.length}
                  </span>
                </h2>
                <p className="text-sm text-neutral-500 dark:text-neutral-400">Partidas abertas na comunidade.</p>
              </div>
              <button
                onClick={() => {
                  const token = session?.accessToken;
                  if (token) fetchRooms(token);
                }}
                className="text-sm text-red-650 dark:text-red-400 hover:text-red-500 dark:hover:text-red-300 font-bold tracking-wide transition-colors flex items-center gap-1 bg-red-500/5 hover:bg-red-500/10 px-2 py-1 rounded-md"
              >
                Atualizar
              </button>
            </div>

            <div className="flex-1 overflow-y-auto space-y-3 pr-1 scrollbar-thin min-h-0">
              {rooms.length === 0 ? (
                <div className="h-full flex flex-col items-center justify-center text-center p-4">
                  <div className="p-3 rounded-full bg-neutral-500/5 mb-3">
                    <svg className="h-8 w-8 text-neutral-400 dark:text-neutral-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 10.5V6.75a4.5 4.5 0 1 1 9 0v3.75M3.75 21.75h16.5a1.5 1.5 0 0 0 1.5-1.5V6.75a1.5 1.5 0 0 0-1.5-1.5H3.75a1.5 1.5 0 0 0-1.5 1.5v13.5a1.5 1.5 0 0 0 1.5 1.5Z" />
                    </svg>
                  </div>
                  <p className="text-base text-neutral-550 font-medium">Nenhuma mesa pública.</p>
                  <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">Inicie a primeira no painel central!</p>
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
                  let statusColor = "bg-neutral-600";
                  let isWaiting = false;

                  if (creatorDisconnected && opponentDisconnected) {
                    roomStatus = "Jogadores reconectando";
                    statusColor = "bg-sky-400 animate-pulse";
                  } else if (creatorDisconnected) {
                    roomStatus = "Dono reconectando";
                    statusColor = "bg-sky-400 animate-pulse";
                  } else if (opponentDisconnected) {
                    roomStatus = "Oponente reconectando";
                    statusColor = "bg-sky-400 animate-pulse";
                  } else if (room.status === "playing") {
                    roomStatus = "Em partida";
                    statusColor = "bg-amber-500";
                  } else if (room.status === "finished") {
                    roomStatus = "Aguardando revanche";
                    statusColor = "bg-red-655";
                  } else if (canPlay) {
                    roomStatus = "Aguardando oponente";
                    statusColor = "bg-red-500";
                    isWaiting = true;
                  }

                  return (
                    <div key={room.id} className="flex flex-col gap-3.5 p-4 rounded-xl bg-neutral-500/[0.02] dark:bg-white/[0.01] border border-neutral-200 dark:border-white/5 hover:bg-neutral-500/[0.04] dark:hover:bg-white/[0.03] hover:border-neutral-350 dark:hover:border-white/10 transition-all duration-200">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2.5">
                          {isWaiting ? (
                            <span className="relative flex h-2.5 w-2.5">
                              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-75"></span>
                              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-red-500"></span>
                            </span>
                          ) : (
                            <span className={`h-2.5 w-2.5 rounded-full ${statusColor}`} />
                          )}
                          <div>
                            <span className="text-xs text-neutral-500 dark:text-neutral-400 uppercase tracking-wider block font-bold">Mesa de</span>
                            <strong className="text-base font-extrabold text-neutral-800 dark:text-white break-words line-clamp-1 max-w-[150px]">{creatorName}</strong>
                          </div>
                        </div>
                        <span className="px-3 py-1 text-sm font-black tracking-widest rounded bg-neutral-500/5 dark:bg-white/5 text-neutral-650 dark:text-neutral-300 border border-neutral-200/50 dark:border-white/10">
                          {room.code}
                        </span>
                      </div>

                      <div className="flex items-center justify-between border-t border-neutral-200/50 dark:border-white/5 pt-3">
                        <span className="text-sm font-semibold text-neutral-600 dark:text-neutral-350">{roomStatus}</span>
                        <div className="flex gap-2">
                          {canReconnect ? (
                            <Button onClick={() => handleWatchPublicRoom(room.id)} variant="solid" className="h-9 px-4 text-xs font-black uppercase tracking-wider bg-sky-600 hover:bg-sky-500 border-sky-600 hover:border-sky-500">
                              Reconectar
                            </Button>
                          ) : (
                            <Button
                              onClick={() => handleJoinPublicRoom(room.id)}
                              disabled={!canPlay}
                              variant={canPlay ? "solid" : "outline"}
                              className="h-9 px-4 text-xs font-black uppercase tracking-wider"
                            >
                              Jogar
                            </Button>
                          )}
                          {!canReconnect && (
                            <Button onClick={() => handleWatchPublicRoom(room.id)} variant="outline" className="h-9 px-4 text-xs font-black uppercase tracking-wider">
                              Assistir
                            </Button>
                          )}
                        </div>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </aside>
        </section>
      </div>
    </main>
  );
}

function sameRoomList(current: Room[], next: Room[]): boolean {
  if (current.length !== next.length) return false;
  return current.every((room, index) => sameRoom(room, next[index]));
}

function sameRoom(current?: Room | null, next?: Room | null): boolean {
  if (!current || !next) return current === next;
  return (
    current.id === next.id &&
    current.code === next.code &&
    current.creator_id === next.creator_id &&
    current.opponent_id === next.opponent_id &&
    current.status === next.status &&
    current.is_private === next.is_private &&
    current.created_at === next.created_at &&
    current.expires_at === next.expires_at &&
    current.creator_disconnected_at === next.creator_disconnected_at &&
    current.opponent_disconnected_at === next.opponent_disconnected_at
  );
}
