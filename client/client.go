package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"time"

	. "github.com/nknorg/nkn-sdk-go"
	"github.com/xtaci/smux"
)

const Topic = "proxyhttp"

var nodeConn net.Conn
var nodeSession *smux.Session
var config = &Configuration{}

type Configuration struct {
	Listener        string `json:"Listener"`
	NodeDialTimeout uint16 `json:"NodeDialTimeout"`
	PrivateKey      string `json:"PrivateKey"`
}

func pipe(dest io.WriteCloser, src io.ReadCloser) {
	defer dest.Close()
	defer src.Close()
	io.Copy(dest, src)
}

func connectToNode(force bool) (net.Conn, error) {
	if nodeConn == nil || force {
		RandomBucket:
		for {
			lastBucket, err := GetTopicBucketsCount(Topic)
			if err != nil {
				return nil, err
			}
			bucket := uint32(rand.Intn(int(lastBucket) + 1))
			subscribers, err := GetSubscribers(Topic, bucket)
			if err != nil {
				return nil, err
			}
			subscribersCount := len(subscribers)

			subscribersIndexes := rand.Perm(subscribersCount)

			RandomSubscriber:
			for {
				if len(subscribersIndexes) == 0 {
					continue RandomBucket
				}
				var subscriberIndex int
				subscriberIndex, subscribersIndexes = subscribersIndexes[0], subscribersIndexes[1:]
				i := 0
				for subscriber, address := range subscribers {
					if i != subscriberIndex {
						i++
						continue
					}

					nodeConn, err = net.DialTimeout("tcp", address, time.Duration(config.NodeDialTimeout)*time.Second)
					if err != nil {
						log.Println("Couldn't connect to address", address, "from", subscriber, "because:", err)
						continue RandomSubscriber
					}

					log.Println("Connected to proxy provider at", address, "from", subscriber)
					break RandomBucket
				}
			}
		}
	}

	return nodeConn, nil
}

func getSession(force bool) (*smux.Session, error) {
	if nodeSession == nil || nodeSession.IsClosed() || force {
		nodeConn, err := connectToNode(force)
		if err != nil {
			return nil, err
		}
		nodeSession, err = smux.Client(nodeConn, nil)
		if err != nil {
			if !force {
				return getSession(true)
			} else {
				return nil, err
			}
		}
	}

	return nodeSession, nil
}

func openStream(force bool) (*smux.Stream, error) {
	nodeSession, err := getSession(force)
	if err != nil {
		return nil, err
	}
	nodeStream, err := nodeSession.OpenStream()
	if err != nil {
		if !force {
			return openStream(true)
		} else {
			return nil, err
		}
	}
	return nodeStream, err
}

func closeConnection(conn net.Conn) {
	err := conn.Close()
	if err != nil {
		log.Println(err)
	}
}

func main() {
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		log.Panicln("Couldn't read config:", err)
	}

	err = json.Unmarshal(file, config)
	if err != nil {
		log.Panicln("Couldn't unmarshal config:", err)
	}

	browserListener, err := net.Listen("tcp", config.Listener)
	if err != nil {
		log.Panicln("Couldn't bind listener:", err)
	}

	Init()

	for {
		browserConn, err := browserListener.Accept()
		if err != nil {
			log.Println("Couldn't accept browser connection:", err)
			closeConnection(browserConn)
			continue
		}

		nodeStream, err := openStream(false)
		if err != nil {
			log.Println("Couldn't open stream:", err)
			closeConnection(browserConn)
			continue
		}

		go pipe(nodeStream, browserConn)
		go pipe(browserConn, nodeStream)
	}
}
