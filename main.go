package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"bufio"

	"io"

	"github.com/gorilla/mux"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	defaultPayloadFilename = "test_payload"
	defaultTimeout         = "5s"
)

var (
	payloadFile       *os.File
	payloadFileLength int64
	outgoingClient    = &http.Client{}
	protocol          = "http"

	//COMMAND LINE STUFF
	cmdline         = kingpin.New("cf-http-payload-tester", "Test your HTTP requests")
	timeout         = cmdline.Flag("timeout", "Time in seconds to wait for response to check calls").Short('t').Default(defaultTimeout).Duration()
	useHTTPS        = cmdline.Flag("https-out", "Use https in outbound URL instead of http").Short('s').Bool()
	payloadFilename = cmdline.Flag("payload", "Target payload file").Short('p').Default(defaultPayloadFilename).String()
)

func main() {
	cmdline.HelpFlag.Short('h')
	kingpin.MustParse(cmdline.Parse(os.Args[1:]))

	err := setup()
	if err != nil {
		log.Fatal(err.Error())
	}

	err = launchAPIServer()
	log.Fatal(err.Error())
}

func setup() (err error) {
	//Get the file to send over HTTP
	payloadFile, err = os.Open(fmt.Sprintf("%s", *payloadFilename))
	if err != nil {
		return fmt.Errorf("Could not open payload file: %s", err.Error())
	}

	//Get length of file for reporting
	stats, err := payloadFile.Stat()
	if err != nil {
		return fmt.Errorf("error stat-ing file: %s", err.Error())
	}

	payloadFileLength = stats.Size()

	//Make sure the PORT env var is set
	if os.Getenv("PORT") == "" {
		return fmt.Errorf("Please set PORT environment variable with port for server to listen on")
	}

	//Make sure that PORT is numeric
	_, err = strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		return fmt.Errorf("PORT environment variable was not numeric")
	}

	log.Printf("Setting HTTP client timeout to %s", *timeout)
	outgoingClient.Timeout = *timeout

	if *useHTTPS {
		protocol = "https"
	}
	log.Printf("Setting protocol to %s", protocol)

	return nil
}

func launchAPIServer() error {
	router := mux.NewRouter()
	router.HandleFunc("/check/{route}", checkHandler).Methods("GET")
	router.HandleFunc("/listen", listenHandler).Methods("POST")

	return http.ListenAndServe(fmt.Sprintf(":%s", os.Getenv("PORT")), router)
}

type responseJSON struct {
	Status       *int   `json:"status,omitempty"`
	ErrorMessage string `json:"error,omitempty"`
	Bytes        *int64 `json:"bytes"`
}

func responsify(r *responseJSON) []byte {
	ret, err := json.Marshal(r)
	if err != nil {
		panic("Couldn't marshal JSON")
	}
	return ret
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	outgoingResp := &responseJSON{Bytes: &payloadFileLength}
	route := mux.Vars(r)["route"]
	resp, err := outgoingClient.Post(fmt.Sprintf("%s://%s/listen", protocol, route), "text/plain", bufio.NewReader(payloadFile))

	//Reset payload file seek position to the start of the file
	defer func() {
		_, err = payloadFile.Seek(0, io.SeekStart)
		if err != nil {
			panic("Could not reset payload file seek position")
		}
	}()

	if err != nil {
		outgoingResp.ErrorMessage = fmt.Sprintf("Error while sending request: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(responsify(outgoingResp))
		return
	}

	//Not sure this can even happen... but...
	if resp.StatusCode != 200 {
		outgoingResp.ErrorMessage = fmt.Sprintf("Non 200-code returned from request to listening server: %d", resp.StatusCode)
	}

	w.WriteHeader(resp.StatusCode)
	outgoingResp.Status = &resp.StatusCode
	w.Write(responsify(outgoingResp))
}

func listenHandler(w http.ResponseWriter, r *http.Request) {
	//I mean... TCP guarantees that if we're this far, the body is correct
	// So.... if we got this far, the payload was successfully sent
	w.WriteHeader(http.StatusOK)
}
