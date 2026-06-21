package nats

import (
	"errors"
	"log"
	"strings"

	"github.com/nats-io/nats.go"
)

var nc *nats.Conn
var js nats.JetStreamContext

func Connect(url string) (*nats.Conn, error) {
	var err error
	nc, err = nats.Connect(url)
	if err != nil {
		return nil, err
	}
	js, err = nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, err
	}
	log.Printf("Conectado ao NATS em %s", url)
	return nc, nil
}

func GetConn() *nats.Conn {
	return nc
}

func GetJetStream() nats.JetStreamContext {
	return js
}

func PublicRoomsSubject() string {
	return "lobby.rooms.public"
}

func OnlineUsersSubject() string {
	return "lobby.users.online"
}

func UserNotificationsSubject(userID string) string {
	return "users." + sanitizeSubjectToken(userID) + ".notifications"
}

func RoomEventsSubject(roomID string) string {
	return "rooms." + sanitizeSubjectToken(roomID) + ".events"
}

func RoomChatSubject(roomID string) string {
	return "rooms." + sanitizeSubjectToken(roomID) + ".chat"
}

func RoomStreamName(roomID string) string {
	return "ROOM_" + strings.NewReplacer("-", "_", ".", "_").Replace(roomID)
}

func sanitizeSubjectToken(token string) string {
	return strings.NewReplacer(".", "_", "*", "_", ">", "_", " ", "_").Replace(token)
}

func EnsureRoomStream(roomID string) error {
	if js == nil {
		return errors.New("jetstream indisponivel")
	}

	streamName := RoomStreamName(roomID)
	if _, err := js.StreamInfo(streamName); err == nil {
		return nil
	} else if !errors.Is(err, nats.ErrStreamNotFound) {
		return err
	}

	_, err := js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{RoomChatSubject(roomID)},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	})
	return err
}

func DeleteRoomStream(roomID string) error {
	if js == nil {
		return nil
	}

	err := js.DeleteStream(RoomStreamName(roomID))
	if errors.Is(err, nats.ErrStreamNotFound) {
		return nil
	}
	return err
}
