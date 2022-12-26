package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	// this has to be the same as the go.mod module,
	// followed by the path to the folder the proto file is in.
	"github.com/mwhaITU/DSYSPrep/ReplicatedChat/proto"
	gRPC "github.com/mwhaITU/DSYSPrep/ReplicatedChat/proto"

	"google.golang.org/grpc"
)

type Server struct {
	gRPC.UnimplementedTemplateServer        // You need this line if you have a server
	name                             string // Not required but useful if you want to name your server
	port                             string // Not required but useful if your server needs to know what port it's listening to

	channel      map[string]chan *proto.Message
	lamportClock int32
	mutex        sync.Mutex // used to lock the server to avoid race conditions.
}

// flags are used to get arguments from the terminal. Flags take a value, a default value and a description of the flag.
// to use a flag then just add it as an argument when running the program.
var serverName = flag.String("name", "default", "Senders name") // set with "-name <name>" in terminal
var port = flag.String("port", "5400", "Server port")           // set with "-port <port>" in terminal

func main() {

	// setLog() //uncomment this line to log to a log.txt file instead of the console

	// This parses the flags and sets the correct/given corresponding values.
	flag.Parse()
	fmt.Println(".:server is starting:.")

	// starts a goroutine executing the launchServer method.
	go launchServer()

	// This makes sure that the main method is "kept alive"/keeps running
	for {
		time.Sleep(time.Second * 5)
	}
}

func launchServer() {
	log.Printf("Server %s: Attempts to create listener on port %s\n", *serverName, *port)

	// Create listener tcp on given port or default port 5400
	list, err := net.Listen("tcp", "localhost:"+*port)
	if err != nil {
		log.Printf("Server %s: Failed to listen on port %s: %v", *serverName, *port, err) //If it fails to listen on the port, run launchServer method again with the next value/port in ports array
		return
	}

	// makes gRPC server using the options
	// you can add options here if you want or remove the options part entirely
	var opts []grpc.ServerOption
	grpcServer := grpc.NewServer(opts...)

	// makes a new server instance using the name and port from the flags.
	server := &Server{
		name:         *serverName,
		port:         *port,
		channel:      make(map[string]chan *proto.Message),
		lamportClock: 0,
	}

	gRPC.RegisterTemplateServer(grpcServer, server) //Registers the server to the gRPC server.

	log.Printf("Server %s: Listening on port %s\n", *serverName, *port)

	if err := grpcServer.Serve(list); err != nil {
		log.Fatalf("failed to serve %v", err)
	}
	for {
		time.Sleep(time.Second * 5)
	}
}

func (s *Server) JoinChat(msg *proto.Message, msgStream proto.Template_JoinChatServer) error {
	msgChannel := make(chan *proto.Message)
	s.channel[msg.Sender] = msgChannel
	for {
		select {
		case <-msgStream.Context().Done():
			return nil
		case msg := <-msgChannel:
			s.lamportClock++
			msg.Lamport = s.lamportClock
			msgStream.Send(msg)
		}
	}
}

func (s *Server) SendMessage(msgStream gRPC.Template_SendMessageServer) error {
	msg, err := msgStream.Recv()

	if err == io.EOF {
		return nil
	}

	if err != nil {
		return err
	}
	if msg.Lamport > s.lamportClock {
		s.lamportClock = msg.Lamport
	}
	log.Printf("Received message: %v \n", msg)
	s.lamportClock++
	if msg.Message == "close" {
		delete(s.channel, msg.Sender)
		log.Printf("Closing connection to client %v", msg.Sender)
		ack := proto.Ack{Acknowledgement: "DISCONNECTED"}
		msgStream.SendAndClose(&ack)
		msg.Message = "Participant " + msg.Sender + " has left Chat at Lamport time " + strconv.Itoa(int(s.lamportClock))
	} else if msg.Message == "join" {
		log.Printf("Participant %v has joined Chat at Lamport time "+strconv.Itoa(int(s.lamportClock)), msg.Sender)
		msg.Message = "Participant " + msg.Sender + " has joined Chat at Lamport time " + strconv.Itoa(int(s.lamportClock))
		ack := proto.Ack{Acknowledgement: "CONNECTED"}
		msgStream.SendAndClose(&ack)
	} else {
		ack := proto.Ack{Acknowledgement: "SENT"}
		msgStream.SendAndClose(&ack)
	}
	go func() {
		for _, msgChan := range s.channel {
			msgChan <- msg
		}
	}()

	return nil
}
