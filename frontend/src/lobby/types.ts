export type RoomStatus = "waiting" | "playing" | "finished" | "expired";

export interface Room {
  id: string;
  code?: string;
  creator_id: string;
  opponent_id?: string;
  status: RoomStatus;
  is_private: boolean;
  created_at: string;
  expires_at: string;
  creator_disconnected_at?: string;
  opponent_disconnected_at?: string;
}

export interface PresenceUser {
  user_id: string;
}

export interface OnlineUsersSnapshot {
  users: PresenceUser[];
  count: number;
}

export interface RoomSpectatorsSnapshot {
  room_id: string;
  spectators: PresenceUser[];
  count: number;
}

export interface CueStatePayload {
  match_id?: string;
  shot_seq: number;
  turn_user_id?: string;
  x: number;
  y: number;
  angle: number;
  power: number;
  is_aiming: boolean;
  client_seq: number;
  server_received_at_ms?: number;
}

export interface MatchFinishedPayload {
  room?: Room;
  reason?: string;
  winner_user_id?: string;
  xp_awards?: XPAward[];
}

export interface XPAward {
  user_id: string;
  xp_delta: number;
  total_xp?: number;
}

export interface RematchRequestedPayload {
  room_id: string;
  user_id: string;
  requested_user_ids?: string[];
}

export interface RoomInvite {
  invitation_id: string;
  room: Room;
  from_user_id: string;
  to_user_id: string;
  created_at: string;
}

export interface RoomInviteCleared {
  invitation_id: string;
  room_id: string;
  from_user_id: string;
  to_user_id: string;
  reason: string;
}

export interface WSEvent<T = any> {
  type: string;
  sender_id?: string;
  payload?: T;
}
