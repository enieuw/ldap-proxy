package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"

	"github.com/patrickmn/go-cache"
)

func main() {
	ln, err := net.Listen("tcp", getEnv("LISTEN_INTERFACE", ":389"))
	if err != nil {
		fmt.Printf("Failed opening socket: %s\n", err)
	}

	//Initialize cache
	c := cache.New(5*time.Minute, 10*time.Minute)

	//Main for loop handling connections
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Printf("Something went wrong accepting the connection: %s\n", err)
		}
		go handleRequest(conn, c)
	}
}

func handleRequest(conn net.Conn, c *cache.Cache) {
	//Reader from source
	reader := bufio.NewReader(conn)
	//List of messages returned to the client
	var messageList []*ber.Packet
	//Is this request cacheable or not? We stop caching the soon we miss once
	var noCacheMisses = true

	//Connection to downstream server
	downstreamConn, err := net.Dial("tcp", getEnv("TARGET_SERVER", "127.0.0.1:389"))
	if err != nil {
		fmt.Printf("Failed opening socket to target: %s\n", err)
	}

	for buf, err := ber.ReadPacket(reader); err == nil; buf, err = ber.ReadPacket(reader) {
		if err != nil {
			fmt.Println("Error reading:", err.Error())
		}

		//Add received package to the stack
		messageList = append(messageList, buf)

		//Generate cacheKey
		hasher := sha1.New()
		hasher.Write(buf.Bytes())
		cacheKey := hex.EncodeToString(hasher.Sum(nil))

		//Check if the packet is in the cache
		var reply *ber.Packet
		if x, found := c.Get(cacheKey); found && noCacheMisses {
			fmt.Printf("CACHE HIT %s \n", cacheKey)

			//Set reply to point to the deserialized packet
			if x != nil {
				replyPkg := x.(ber.Packet)
				reply = &replyPkg
			}
		} else {
			fmt.Printf("CACHE MISS %s \n", cacheKey)

			//If we have one cache miss stop caching for this request and replay all previously sent messages for this request
			if noCacheMisses && len(messageList) > 1 {
				noCacheMisses = false
				for _, element := range messageList[:len(messageList)-1] {
					_ = forwardRequest(downstreamConn, element)
				}
			}

			//Forward the request to the downstream server
			reply = forwardRequest(downstreamConn, buf)

			//On closing the connection the original server does not return a reply so we cache a nil
			if reply != nil {
				c.Set(cacheKey, *reply, cache.DefaultExpiration)
			} else {
				c.Set(cacheKey, nil, cache.DefaultExpiration)
			}
		}

		//If we received a reply write it back to the client
		if reply != nil {
			conn.Write(reply.Bytes())
		}
	}

	conn.Close()
}

func forwardRequest(conn net.Conn, buffer *ber.Packet) *ber.Packet {
	//Forward request to downstream connection
	conn.Write(buffer.Bytes())

	//Fetch reply if any
	replyReader := bufio.NewReader(conn)
	replyBuf, err := ber.ReadPacket(replyReader)
	if err != nil {
		fmt.Println(err)
	}

	return replyBuf
}
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
