package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"whapp-irc/whapp"
)

func formatContact(contact whapp.Contact, isAdmin bool) Participant {
	return Participant{
		ID:      contact.ID,
		IsAdmin: isAdmin,
		Contact: contact,
	}
}

func getMessageBody(msg whapp.Message, participants []Participant, me whapp.Me) string {
	whappParticipants := make([]whapp.Participant, len(participants))
	for i, p := range participants {
		whappParticipants[i] = whapp.Participant(p)
	}

	if msg.Location != nil {
		return fmt.Sprintf(
			"https://maps.google.com/?q=%f,%f",
			msg.Location.Latitude,
			msg.Location.Longitude,
		)
	} else if msg.IsMMS {
		res := "--file--"
		if f, has := fs.HashToPath[msg.MediaFileHash]; has {
			res = f.URL
		}

		if msg.Caption != "" {
			res += " " + msg.FormatCaption(whappParticipants, me.Pushname)
		}

		return res
	}

	return msg.FormatBody(whappParticipants, me.Pushname)
}

func downloadAndStoreMedia(msg whapp.Message) error {
	if _, ok := fs.HashToPath[msg.MediaFileHash]; msg.IsMMS && !ok {
		bytes, err := msg.DownloadMedia()
		if err != nil {
			return err
		}

		ext := getExtensionByMimeOrBytes(msg.MimeType, bytes)
		if ext == "" {
			ext = filepath.Ext(msg.MediaFilename)
			if ext != "" {
				ext = ext[1:]
			}
		}

		if _, err := fs.AddBlob(
			msg.MediaFileHash,
			ext,
			bytes,
		); err != nil {
			return err
		}
	}

	return nil
}

func (conn *Connection) handleWhappMessage(msg whapp.Message) error {
	// HACK
	if msg.Type == "e2e_notification" {
		return nil
	}

	var err error

	chat := conn.GetChatByID(msg.Chat.ID)
	if chat == nil {
		chat, err = conn.addChat(msg.Chat)
		if err != nil {
			return err
		}
	}

	if chat.IsGroupChat && !chat.Joined {
		if err := conn.joinChat(chat); err != nil {
			return err
		}
	}

	if chat.HasMessageID(msg.ID.Serialized) {
		return nil // already handled
	}
	chat.AddMessageID(msg.ID.Serialized)

	lastTimestamp, found := conn.timestampMap.Get(chat.ID)
	if !found || msg.Timestamp > lastTimestamp {
		conn.timestampMap.Set(chat.ID, msg.Timestamp)
		go conn.saveDatabaseEntry()
	}

	if msg.IsNotification {
		return conn.handleWhappNotification(chat, msg)
	}

	sender := formatContact(*msg.Sender, false)
	senderSafeName := sender.SafeName()

	if msg.IsSentByMeFromWeb {
		return nil
	} else if msg.IsSentByMe {
		senderSafeName = conn.nickname
	}

	var to string
	if chat.IsGroupChat || msg.IsSentByMe {
		to = chat.Identifier()
	} else {
		to = conn.nickname
	}

	if err := downloadAndStoreMedia(msg); err != nil {
		return err
	}

	if msg.QuotedMessageObject != nil {
		message := getMessageBody(*msg.QuotedMessageObject, chat.Participants, conn.me)
		line := "> " + strings.SplitN(message, "\n", 2)[0]
		str := formatPrivateMessage(senderSafeName, to, line)
		conn.writeIRC(msg.Time(), str)
	}

	message := getMessageBody(msg, chat.Participants, conn.me)
	for _, line := range strings.Split(message, "\n") {
		logMessage(msg.Time(), senderSafeName, to, line)
		str := formatPrivateMessage(senderSafeName, to, line)
		conn.writeIRC(msg.Time(), str)
	}

	return nil
}

func (conn *Connection) handleWhappNotification(chat *Chat, msg whapp.Message) error {
	if msg.Type != "gp2" {
		return fmt.Errorf("no idea what to do with notification type %s", msg.Type)
	} else if len(msg.RecipientIDs) == 0 {
		return nil
	}

	findName := func(id string) string {
		for _, p := range chat.Participants {
			if p.ID == id {
				return p.SafeName()
			}
		}

		if chat := conn.GetChatByID(id); chat != nil && !chat.IsGroupChat {
			return chat.Identifier()
		}

		return strings.Split(id, "@")[0]
	}

	if msg.Sender != nil {
		msg.From = msg.Sender.ID
	}

	var author string
	if msg.From == conn.me.SelfID {
		author = conn.nickname
	} else {
		author = findName(msg.From)
	}

	for _, recipientID := range msg.RecipientIDs {
		recipientSelf := recipientID == conn.me.SelfID
		var recipient string
		if recipientSelf {
			recipient = conn.nickname
		} else {
			recipient = findName(recipientID)
		}

		switch msg.Subtype {
		case "create":
			break

		case "add", "invite":
			if recipientSelf {
				// We already handle the new chat JOIN in
				// `Connection::handleWhappMessage` in a better way.
				// So just skip this, since otherwise we JOIN double.
				break
			}
			conn.writeIRC(msg.Time(), fmt.Sprintf(":%s JOIN %s", recipient, chat.Identifier()))

		case "leave":
			conn.writeIRC(msg.Time(), fmt.Sprintf(":%s PART %s", recipient, chat.Identifier()))

		case "remove":
			conn.writeIRC(msg.Time(), fmt.Sprintf(":%s KICK %s %s", author, chat.Identifier(), recipient))

		default:
			log.Printf("no idea what to do with notification subtype %s\n", msg.Subtype)
		}

		if recipientSelf && (msg.Subtype == "leave" || msg.Subtype == "remove") {
			chat.Joined = false
		}
	}

	return nil
}
