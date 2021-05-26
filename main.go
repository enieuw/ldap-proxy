package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"

	"github.com/patrickmn/go-cache"
)

func main() {
	ln, err := net.Listen("tcp", getEnv("LISTEN_INTERFACE", ":389"))
	if err != nil {
		fmt.Printf("Failed opening socket: %s\n", err)
	}

	cacheDuration, err := strconv.Atoi(getEnv("CACHE_DURATION_MINUTES", "15"))
	if err != nil {
		fmt.Printf("Error converting cache duration: %s\n", err)
	}

	//Initialize cache
	c := cache.New(time.Duration(cacheDuration)*time.Minute, time.Duration(2*cacheDuration)*time.Minute)

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
	//Is this request cacheable or not? We stop caching for this request as soon we miss once
	var noCacheMisses = true
	var downstreamConn net.Conn

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
		var replies []ber.Packet
		if x, found := c.Get(cacheKey); found && noCacheMisses {
			fmt.Printf("CACHE HIT %s \n", cacheKey)

			//Set replies to point to the deserialized packet
			if x != nil {
				replyPkg := x.([]ber.Packet)
				replies = replyPkg
			}
		} else {
			fmt.Printf("CACHE MISS %s \n", cacheKey)

		    //Only initialize the downstream connection after a cache miss
			if downstreamConn == nil {
				//Connection to downstream server
				downstreamConn, err = net.Dial("tcp", getEnv("TARGET_SERVER", "127.0.0.1:389"))
				if err != nil {
					fmt.Printf("Failed opening socket to target: %s\n", err)
				}
			}

			//If we have one cache miss stop caching for this request and replay all previously sent messages for this request
			if noCacheMisses && len(messageList) > 1 {
				noCacheMisses = false
				for _, element := range messageList[:len(messageList)-1] {
					//We ignore these replies as they have already been cached
					_ = forwardRequest(downstreamConn, element)
				}
			}

			//Forward the request to the downstream server
			replies = forwardRequest(downstreamConn, buf)

			//On closing the connection the original server does not return a reply so we cache a nil
			if replies != nil {
				c.Set(cacheKey, replies, cache.DefaultExpiration)
			} else {
				c.Set(cacheKey, nil, cache.DefaultExpiration)
			}
		}

		//If we received one or more replies write them back to the client
		if replies != nil {
			for _, element := range replies {
				conn.Write(element.Bytes())
			}
		}
	}

	if downstreamConn != nil {
		downstreamConn.Close()
	}

	conn.Close()
}

func forwardRequest(conn net.Conn, buffer *ber.Packet) []ber.Packet {
	//List of messages returned to the client
	var packetList []ber.Packet

	//Forward request to downstream connection
	if buffer != nil {
		conn.Write(buffer.Bytes())
	}
	//Fetch reply if any
	replyReader := bufio.NewReader(conn)
	replyBuf, err := ber.ReadPacket(replyReader)
	if err != nil {
		fmt.Println(err)
	}

	//Sometimes there is more than one packet hidden inside
	if replyBuf != nil {
		packetList = append(packetList, *replyBuf)

		//TODO: add error handling here
		//If this field is set to 4 it means there's more to be had
		morePackagesRemaining := replyBuf.Children[1].Tag == 4

		for morePackagesRemaining == true {
			extraReply, err := ber.ReadPacket(replyReader)
			if err != nil {
				fmt.Println(err)
			}

			packetList = append(packetList, *extraReply)
			morePackagesRemaining = extraReply.Children[1].Tag == 4
		}
	}

	return packetList
}
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
