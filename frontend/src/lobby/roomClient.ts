const ROOM_CLIENT_ID_KEY = "snooker.room_client_id";

let fallbackRoomClientId = "";

export function getRoomClientId(): string {
  try {
    const existing = window.sessionStorage.getItem(ROOM_CLIENT_ID_KEY);
    if (existing) return existing;

    const created = createRoomClientId();
    window.sessionStorage.setItem(ROOM_CLIENT_ID_KEY, created);
    return created;
  } catch {
    if (!fallbackRoomClientId) {
      fallbackRoomClientId = createRoomClientId();
    }
    return fallbackRoomClientId;
  }
}

function createRoomClientId(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `room-client-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}
