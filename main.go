package main

import (
	"bytes"
	ctx "context"
	"fmt"
	"image/jpeg"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chai2010/webp"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal"
	"go.mau.fi/whatsmeow"
	wp "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var client *whatsmeow.Client

func main() {
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New("sqlite3", "file:whatgpt.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		panic(err)
	}
	clientLog := waLog.Stdout("Client", "INFO", true)
	client = whatsmeow.NewClient(deviceStore, clientLog)

	client.AddEventHandler(func(evt interface{}) {
		if evt, ok := evt.(*events.Message); ok {
			if evt.Message.ImageMessage != nil {
				imageData, _ := client.Download(evt.Message.GetImageMessage())

				img, _ := jpeg.Decode(bytes.NewReader(imageData))

				var buf bytes.Buffer
				err = webp.Encode(&buf, img, &webp.Options{Lossless: false})

				if err != nil {
					panic("Failid to download from whatsapp server" + err.Error())
				}

				uploadedImage, err := client.Upload(ctx.Background(), buf.Bytes(), whatsmeow.MediaImage)
				if err != nil {
					panic("Failid to Upload on whatsapp server" + err.Error())
				}

				_, err = client.SendMessage(ctx.Background(), evt.Info.Chat, &wp.Message{
					StickerMessage: &wp.StickerMessage{
						Url:               proto.String(uploadedImage.URL),
						DirectPath:        proto.String(uploadedImage.DirectPath),
						MediaKey:          uploadedImage.MediaKey,
						MediaKeyTimestamp: proto.Int64(time.Now().Unix()),
						Mimetype:          proto.String(http.DetectContentType(buf.Bytes())),
						FileEncSha256:     uploadedImage.FileEncSHA256,
						FileSha256:        uploadedImage.FileSHA256,
						FileLength:        proto.Uint64(uint64(len(buf.Bytes()))),
						Height:            proto.Uint32(uint32(evt.Message.GetImageMessage().GetHeight())),
						Width:             proto.Uint32(uint32(evt.Message.GetImageMessage().GetWidth())),
					},
				})

				if err != nil {
					panic(err)
				}
			}
		}
	})

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(ctx.Background())
		// Connect to WhatsApp
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		// Print the QR code to the console
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Connect to WhatsApp if already logged in
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
