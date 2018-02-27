package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/go-zoo/bone"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var rxChan = make(chan string)
var txChan = make(chan string)

func main() {
	fmt.Println("Hello")
	go watchCommand()

	mux := bone.New()
	//create the http endpoints
	mux.Get("/", http.HandlerFunc(Home))
	mux.Get("/ws", http.HandlerFunc(WebSocket))
	//	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.Handle("/static/", http.HandlerFunc(staticFiles))
	//create a middleware
	n := negroni.Classic()
	n.UseHandler(mux)
	n.Run(":8000")

}

func watchCommand() {
	type Credential struct {
		IP       string `json:"ipaddress"`
		Username string `json:"username"`
		Password string `json:"password"`
		Command  string `json:"command"`
	}

	go func() {
		for {
			time.Sleep(50 * time.Millisecond)
			m := <-txChan
			data := &Credential{}
			err := json.Unmarshal([]byte(m), data)
			if err != nil {
				panic(err)
			}

			go connect(data.IP, data.Username, data.Password, data.Command)
		}
	}()
}

func staticFiles(w http.ResponseWriter, r *http.Request) {
	contentType := ""
	path := r.URL.Path[1:]
	if filepath.Ext(path) == ".css" {
		contentType = "text/css"
	} else if filepath.Ext(path) == ".js" {
		contentType = "text/javascript"
	}

	fmt.Println("Pat: " + path)
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		w.Header().Add("Content-Type", contentType)
		br := bufio.NewReader(f)
		br.WriteTo(w)
	} else {
		w.WriteHeader(404)
	}
}

func connect(ipAddress string, user string, pwd string, cmd string) {
	var command string
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pwd),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", ipAddress+":22", config)
	if err != nil {
		panic(err)
	}
	switch cmd {
	case "cpuinfo":
		command = "cat /proc/cpuinfo"
		break

	case "memory":
		command = "cat /proc/meminfo"
		break

	case "disk":
		command = "/bin/df -h"
		break
	}

	session, err := client.NewSession()
	if err != nil {
		panic(err)
	}

	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(command); err != nil {
		panic(err)
	}
	rxChan <- b.String()

}

/*
  Home - handler for serving the index.html file
*/
func Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Method not found", 404)
		return
	}

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	f, err := os.Open("index.html")
	if err == nil {
		defer f.Close()
		w.Header().Add("Content-Type", "text/html")
		br := bufio.NewReader(f)
		br.WriteTo(w)
	} else {
		fmt.Println(err)
		w.WriteHeader(404)
	}
}

/*
  WebSocket - handler for the websocket
*/
func WebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)

	if err != nil {
		log.Println("Upgrade Error: ", err)
		return
	}

	//create goroutines for incoming data from Pi
	go wsFromPi(ws)
	go wsGetMsg(ws)
}

/*
  wsSendMsg - Send message to the WebSocket
  params - ws connection
*/
func wsFromPi(ws *websocket.Conn) {
	for {
		select {

		case m := <-rxChan:
			ws.WriteMessage(websocket.TextMessage, []byte(m))
			//fmt.Println(m)
			// case <-time.After(60000 * time.Millisecond):
			// 	m := "console timeout...\n"
			//ws.WriteMessage(websocket.TextMessage, []byte(m))
			// 	txChan <- "exit\n"

		}
	}
}

/*
  wsGetMasg - Read message from the websocket
  params - ws connection
*/
func wsGetMsg(ws *websocket.Conn) {
	for {
		time.Sleep(50 * time.Millisecond)
		_, msg, err := ws.ReadMessage()

		if err != nil {
			fmt.Printf("wsGetMsg Error: %s\n", err)
			break
		}
		//fmt.Printf("%s\n", msg)
		txChan <- string(msg)
	}
	defer ws.Close()
}
