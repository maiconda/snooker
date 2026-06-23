import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";
import { navigate } from "../lib/router";
import { getPublicProfile } from "../profile/profileApi";
import type { Profile } from "../profile/types";
import { declineInvite, joinRoom } from "./lobbyApi";
import type { OnlineUsersSnapshot, PresenceUser, RoomInvite, RoomInviteCleared, WSEvent } from "./types";

type LobbyNotificationsContextValue = {
  onlineUsers: PresenceUser[];
  invites: RoomInvite[];
  clearedInvites: RoomInviteCleared[];
  acceptInvite: (invite: RoomInvite) => Promise<void>;
  dismissInvite: (invite: RoomInvite) => Promise<void>;
};

const LobbyNotificationsContext = createContext<LobbyNotificationsContextValue | null>(null);

export function LobbyNotificationsProvider({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const token = auth.session?.accessToken;
  const isAuthenticated = auth.phase === "authenticated" && auth.session?.status !== "blocked";
  const [onlineUsers, setOnlineUsers] = useState<PresenceUser[]>([]);
  const [invites, setInvites] = useState<RoomInvite[]>([]);
  const [clearedInvites, setClearedInvites] = useState<RoomInviteCleared[]>([]);
  const [profilesById, setProfilesById] = useState<Record<string, Profile>>({});

  const rememberClearedInvite = useCallback((cleared: RoomInviteCleared) => {
    setClearedInvites((prev) => [...prev.slice(-49), cleared]);
  }, []);

  useEffect(() => {
    if (!isAuthenticated || !token) {
      setOnlineUsers([]);
      setInvites([]);
      return;
    }

    let active = true;
    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      ws = new WebSocket(`${protocol}//${window.location.host}/api/v1/notifications/ws?token=${token}`);

      ws.onmessage = (event) => {
        try {
          const wsMsg = JSON.parse(event.data) as WSEvent<OnlineUsersSnapshot | RoomInvite | RoomInviteCleared>;
          if (!active || !wsMsg.payload) return;

          if (wsMsg.type === "online_users_snapshot" && "users" in wsMsg.payload) {
            const snapshot = wsMsg.payload as OnlineUsersSnapshot;
            setOnlineUsers(Array.isArray(snapshot.users) ? snapshot.users : []);
            return;
          }

          if (wsMsg.type === "room_invite" && "room" in wsMsg.payload) {
            const invite = wsMsg.payload as RoomInvite;
            setInvites((prev) => [
              ...prev.filter((item) => item.invitation_id !== invite.invitation_id && item.room.id !== invite.room.id),
              invite
            ]);
            return;
          }

          if (wsMsg.type === "room_invite_cleared" && "room_id" in wsMsg.payload) {
            const cleared = wsMsg.payload as RoomInviteCleared;
            rememberClearedInvite(cleared);
            setInvites((prev) => prev.filter((item) => item.invitation_id !== cleared.invitation_id && item.room.id !== cleared.room_id));
          }
        } catch (err) {
          console.error("Erro ao processar notificacao do lobby:", err);
        }
      };

      ws.onclose = () => {
        if (!active) return;
        reconnectTimer = setTimeout(connect, 2000);
      };
    };

    connect();

    return () => {
      active = false;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      ws?.close();
    };
  }, [isAuthenticated, rememberClearedInvite, token]);

  useEffect(() => {
    if (!token) return;
    let active = true;

    invites.forEach((invite) => {
      if (profilesById[invite.from_user_id]) return;
      getPublicProfile(token, invite.from_user_id)
        .then((profile) => {
          if (!active) return;
          setProfilesById((prev) => ({ ...prev, [invite.from_user_id]: profile }));
        })
        .catch((err) => console.error("Erro ao buscar apelido do convite:", err));
    });

    return () => {
      active = false;
    };
  }, [invites, profilesById, token]);

  const acceptInvite = useCallback(async (invite: RoomInvite) => {
    if (!token) return;
    try {
      const joined = await joinRoom(token, invite.room.id);
      setInvites((prev) => prev.filter((item) => item.invitation_id !== invite.invitation_id));
      navigate(`/sala/${joined.id}`);
    } catch {
      setInvites((prev) => prev.filter((item) => item.invitation_id !== invite.invitation_id));
    }
  }, [token]);

  const dismissInvite = useCallback(async (invite: RoomInvite) => {
    if (!token) return;
    setInvites((prev) => prev.filter((item) => item.invitation_id !== invite.invitation_id));
    try {
      await declineInvite(token, invite.invitation_id);
    } catch (err) {
      console.error("Erro ao recusar convite:", err);
    }
  }, [token]);

  const value = useMemo<LobbyNotificationsContextValue>(() => ({
    onlineUsers,
    invites,
    clearedInvites,
    acceptInvite,
    dismissInvite
  }), [acceptInvite, clearedInvites, dismissInvite, invites, onlineUsers]);

  return (
    <LobbyNotificationsContext.Provider value={value}>
      {children}
      {isAuthenticated && invites.length > 0 && (
        <div className="fixed right-4 top-4 z-50 w-[min(360px,calc(100vw-2rem))] space-y-3">
          {invites.map((invite) => (
            <div key={invite.invitation_id} className="rounded-lg border border-neutral-200 dark:border-red-500/30 bg-white/95 dark:bg-neutral-950 p-4 text-neutral-900 dark:text-white shadow-2xl shadow-neutral-200/40 dark:shadow-black/30">
              <p className="text-xs uppercase tracking-[0.16em] text-red-600 dark:text-red-400 font-bold">Convite recebido</p>
              <p className="mt-2 text-sm text-neutral-600 dark:text-neutral-200">
                {profilesById[invite.from_user_id]?.nickname ?? "Um jogador"} te convidou para uma mesa.
              </p>
              <div className="mt-3 flex gap-2">
                <Button onClick={() => acceptInvite(invite)} className="flex-1 px-3 py-1.5 text-xs">
                  Entrar
                </Button>
                <Button onClick={() => dismissInvite(invite)} variant="outline" className="px-3 py-1.5 text-xs">
                  Negar
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </LobbyNotificationsContext.Provider>
  );
}

export function useLobbyNotifications() {
  const context = useContext(LobbyNotificationsContext);
  if (!context) {
    throw new Error("useLobbyNotifications deve ser usado dentro de LobbyNotificationsProvider.");
  }
  return context;
}
