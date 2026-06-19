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
}

export interface WSEvent<T = any> {
  type: string;
  sender_id?: string;
  payload?: T;
}
