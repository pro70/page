package main

import (
	"net/http"
	"os"
	"sync"
	"time"
	"log"
	"encoding/json"

	"github.com/julienschmidt/httprouter"
	"github.com/kardianos/service"
	"github.com/julienschmidt/sse"
)

const serviceName = "Pro70 Page Service"
const serviceDescription = "Multi-User-Terminal-Simulation"

const rowLimit = 30
const lineLimit = 80
const limit = rowLimit * lineLimit

var (
	serviceIsRunning bool
	programIsRunning bool
	writingSync      sync.Mutex
	events           chan Event
	contentData      [][]KeyPressEvent
	currentRow       int
)

type program struct{}

type KeyPressEvent struct {
	Key   string
	Color string
}

type Event struct {
	ID    string
	Event string
	Data  interface{}
}


func (p program) Start(s service.Service) error {
	log.Println("SERVICE", s.String(), "started")

	writingSync.Lock()
	serviceIsRunning = true
	writingSync.Unlock()

	go p.run()
	return nil
}

func (p program) Stop(s service.Service) error {
	writingSync.Lock()
	serviceIsRunning = false
	writingSync.Unlock()

	close(events)

	for programIsRunning {
		log.Println("SERVICE", s.String(), "stopping...")
		time.Sleep(1 * time.Second)
	}

	log.Println("SERVICE", s.String(), "stopped")
	return nil
}

func (p program) run() {
	events = make(chan Event, 100)
	currentRow = 0
	contentData = make([][]KeyPressEvent, 1)
	contentData[0] = make([]KeyPressEvent, 0)

	router := httprouter.New()
	sender := sse.New()

	router.Handler("GET", "/api/contentUpdate", sender)
	router.GET("/api/content", contentEndpoint)

    go streamEvents(sender)

	router.POST("/api/keyPress", keyPressEndpoint)

	err := http.ListenAndServe(":6080", router)
	if err != nil {
		log.Println("RUN", "Problem starting web server:", err.Error())
		os.Exit(-1)
	}
}

func contentEndpoint(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
	timer := time.Now()

	log.Println("CLIENT", "get content", request.RemoteAddr)

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(200)
	
	err := json.NewEncoder(writer).Encode(contentData)
	if err != nil {
		log.Println("CLIENT", "response encode error", err.Error())
	}

	end := time.Since(timer)
	log.Println("CLIENT", request.RemoteAddr, "processing takes:", end.String())
}

func keyPressEndpoint(writer http.ResponseWriter, request *http.Request, params httprouter.Params) {
	var data KeyPressEvent
	timer := time.Now()

	err := json.NewDecoder(request.Body).Decode(&data)
	if err != nil {
		log.Println("CLIENT", "request data error", request.RemoteAddr, err)
		writer.Header().Set("Content-Type", "application/json")
		return
	}

	log.Println("CLIENT", "key press", data, request.RemoteAddr)

	if data.Key == "\n" {
		newRow := make([]KeyPressEvent, 0)

		writingSync.Lock()
		contentData = append(contentData, newRow)
		contentLen := len(contentData)
		if contentLen > rowLimit {
			offset := contentLen - rowLimit
			contentData = contentData[offset:]
		}
		currentRow = len(contentData) - 1
		writingSync.Unlock()

		events <- Event{"", "UPDATE_ALL", contentData}
	} else {
		writingSync.Lock()
		contentData[currentRow] = append(contentData[currentRow], data)
		writingSync.Unlock()

		events <- Event{"", "UPDATE_ROW", contentData[currentRow]}
	}

	end := time.Since(timer)
	log.Println("CLIENT", request.RemoteAddr, "processing takes:", end.String())

	writer.Header().Set("Content-Type", "application/json")
}


func streamEvents(sender *sse.Streamer) {
	log.Println("SSE", "Streaming events started")
	for e := range events {
		log.Println("SSE", "Send event", e)
		sender.SendJSON(e.ID, e.Event, e.Data)
	}
	log.Println("SSE", "Streaming events stopped")
}


func main() {
	serviceConfig := &service.Config{
		Name:        serviceName,
		DisplayName: serviceName,
		Description: serviceDescription,
	}

	prg := &program{}

	s, err := service.New(prg, serviceConfig)
	if err != nil {
		log.Println("MAIN", "Cannot create the service: ", err.Error())
	}
	err = s.Run()
	if err != nil {
		log.Println("MAIN", "Cannot start the service: ", err.Error())
	}
}

