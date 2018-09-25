package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/juju2013/go-freebox"
	"github.com/yosssi/gmq/mqtt"
	"github.com/yosssi/gmq/mqtt/client"
)

var fbx *freebox.Client
var cli *client.Client
var sigc chan os.Signal

const TOP_JOIN = "landline/mqtt-free"
const TOP_CALL = "landline/gaston"

var SMS_LOGIN, SMS_PASS string

func main() {
	// Set up channel on which to send signal notifications.
	sigc = make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill)
	if os.Getenv("DEBUG") != "" {
		log.SetLevel(log.DebugLevel)
	}
	SMS_LOGIN = os.Getenv("GOFBX_SMS_LOGIN")
	SMS_PASS = os.Getenv("GOFBX_SMS_PASS")

	initMqtt()
	log.Info("Mqtt ... OK")
	defer cli.Terminate()
	publish(TOP_JOIN, "join")

	initFreebox()

	// Wait for receiving a signal.
	log.Info("Initialized, going to main loop")
Loop:
	for {
		// new call ?
		caller, new := checkCall()
		if new {
			log.WithFields(log.Fields{"who": caller.Name, "when": caller.Datetime}).Info("New call")
			if caller.ContactID > 0 {
				publish(TOP_CALL, caller.Name)
				notifySMS(caller.Name)
			}
			fbx.MarkRead(caller.ID)
		}
		// continue or stop ?
		select {
		case <-sigc:
			log.Info("Exiting...")
			break Loop
		default:
			time.Sleep(time.Second)
		}
	}

	// Disconnect the Network Connection.
	if err := cli.Disconnect(); err != nil {
		panic(err)
	}
}

func initMqtt() {
	// Create an MQTT Client.
	cli = client.New(&client.Options{
		// Define the processing of the error handler.
		ErrorHandler: func(err error) {
			log.Fatal(err)
		},
	})
	// Connect to the MQTT Server.
	err := cli.Connect(&client.ConnectOptions{
		Network:  "tcp",
		Address:  "192.168.88.2:1883",
		ClientID: []byte("mqtt-freebox"),
	})
	if err != nil {
		log.Fatal(err)
	}

}

// send a message
func publish(topic, message string) error {
	// Publish a message.
	err := cli.Publish(&client.PublishOptions{
		QoS:       mqtt.QoS0,
		TopicName: []byte(topic),
		Message:   []byte(message),
	})
	if err != nil {
		log.Warn(err)
	}
	return err
}

func initFreebox() {
	fbx = freebox.New()

	err := fbx.Connect()
	if err != nil {
		log.Fatalf("fbx.Connect(): %v", err)
	}

	err = fbx.Authorize()
	if err != nil {
		log.Fatalf("fbx.Authorize(): %v", err)
	}

	err = fbx.Login()
	if err != nil {
		log.Fatalf("fbx.Login(): %v", err)
	}
}

// check if there's incomming call
func checkCall() (*freebox.CallEntry, bool) {
	calls, err := fbx.GetCallEntries()
	if err != nil {
		return nil, false
	}
	for _, c := range calls {
		if (c.Type == "missed") && (c.New) {
			return &c, true
		}
	}
	return nil, false
}

// notify by sms
func notifySMS(msg string) {
	if (SMS_LOGIN == "") || (SMS_PASS) == "" {
		return
	}

  data := url.Values{
		"user": {SMS_LOGIN},
		"pass": {SMS_PASS},
		"msg":  {msg}}
	response, err := http.Get("https://smsapi.free-mobile.fr/sendmsg?"+data.Encode())
  fmt.Printf("DEBUG:data=%v",data.Encode())

	if err != nil {
		log.Warn(response)
	}

	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)

  fmt.Printf("DEBUG:body=%v",body)
	if err != nil {
		log.Warn(response)
	} else {
		log.WithFields(log.Fields{"http status": response.Status}).Info("Seding SMS ...")
	}
}
