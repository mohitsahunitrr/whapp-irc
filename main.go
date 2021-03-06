package main

import (
	"log"
	"net"
	"os"
	"whapp-irc/database"
)

const (
	defaultHost           = "localhost"
	defaultFileServerPort = "3000"
	defaultIRCPort        = "6060"
)

var fs *FileServer
var userDb *database.Database

func handleSocket(socket *net.TCPConn) {
	conn, err := MakeConnection()
	if err != nil {
		log.Printf("error while making connection: %s", err)
	}
	go conn.BindSocket(socket)
}

func main() {
	host := os.Getenv("HOST")
	if host == "" {
		host = defaultHost
	}

	fileServerPort := os.Getenv("FILE_SERVER_PORT")
	if fileServerPort == "" {
		fileServerPort = defaultFileServerPort
	}

	ircPort := os.Getenv("IRC_SERVER_PORT")
	if ircPort == "" {
		ircPort = defaultIRCPort
	}

	var err error

	userDb, err = database.MakeDatabase("db/users")
	if err != nil {
		panic(err)
	}

	fs, err = MakeFileServer(host, fileServerPort, "files")
	if err != nil {
		panic(err)
	}
	go fs.Start()
	defer fs.Stop()

	addr, err := net.ResolveTCPAddr("tcp", ":"+ircPort)
	if err != nil {
		panic(err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		panic(err)
	}

	for {
		socket, err := listener.AcceptTCP()
		if err != nil {
			log.Printf("error accepting TCP connection: %#v", err)
			continue
		}

		go handleSocket(socket)
	}
}
