package app

import (
	"bytes"
	"fmt"
	"github.com/Tapjoy/riakQueue/app/config"
	"github.com/go-martini/martini"
	"github.com/hashicorp/memberlist"
	"github.com/martini-contrib/render"
	"log"
	"net/http"
	"strconv"
)

//TODO make message definitions more explicit

func InitWebserver(list *memberlist.Memberlist, cfg config.Config) {
	//init the connectionPool
	riakPool := config.InitRiakPool(cfg)
	// tieing our Queue to HTTP interface == bad we should move this somewhere else
	queues := InitQueues(riakPool)
	// also tieing topics this is next for refactor
	topics := InitTopics(cfg, riakPool, queues)
	m := martini.Classic()
	m.Use(render.Renderer())

	// STATUS / STATISTICS API BLOCK
	m.Get("/status/servers", func() string {
		return_string := ""
		for _, member := range list.Members() {
			return_string += fmt.Sprintf("Member: %s %s\n", member.Name, member.Addr)
		}
		return return_string
	})
	// END STATUS / STATISTICS API BLOCK

	// CONFIGURATION API BLOCK
	m.Put("/topics/:topic", func(r render.Render, params martini.Params) {

	})

	m.Delete("/topics/:topic", func(r render.Render, params martini.Params) {

		var present bool
		_, present = topics.TopicMap[params["topic"]]
		if present != true {
			topics.InitTopic(params["topic"])
		}
		r.JSON(200, map[string]interface{}{"Deleted": topics.DeleteTopic(params["topic"])})
	})

	m.Put("/queues/:queue", func(r render.Render, params martini.Params) {
		cfg.InitializeQueue(params["queue"])
		r.JSON(200, "ok")
	})

	m.Put("/topics/:topic/queues/:queue", func(r render.Render, params martini.Params) {
		var present bool
		_, present = topics.TopicMap[params["topic"]]
		if present != true {
			topics.InitTopic(params["topic"])
		}
		topics.TopicMap[params["topic"]].AddQueue(params["queue"])
		r.JSON(200, map[string]interface{}{"Queues": topics.TopicMap[params["topic"]].ListQueues()})
	})

	m.Delete("/queues/:queue", func(r render.Render, params martini.Params) {

	})

	// neeeds a little work....
	m.Delete("/topics/:topic/queues/:queue", func(r render.Render, params martini.Params) {
		var present bool
		_, present = topics.TopicMap[params["topic"]]
		if present != true {
			topics.InitTopic(params["topic"])
		}
		topics.TopicMap[params["topic"]].DeleteQueue(params["queue"])
		r.JSON(200, map[string]interface{}{"Queues": topics.TopicMap[params["topic"]].ListQueues()})
	})

	m.Patch("/topics/:topic/queues/:queue", func(r render.Render, params martini.Params) {

	})

	m.Patch("/queues/:queue", func(r render.Render, params martini.Params) {

	})

	// END CONFIGURATION API BLOCK

	// DATA INTERACTION API BLOCK

	m.Get("/topics", func(r render.Render) {
		topicList := make([]string, 0, 10)
		for topicName, _ := range topics.TopicMap {
			topicList = append(topicList, topicName)
		}
		r.JSON(200, map[string]interface{}{"topics": topicList})
	})

	m.Get("/topics/:topic", func(r render.Render, params martini.Params) {
		var present bool
		_, present = topics.TopicMap[params["topic"]]
		if present != true {
			topics.InitTopic(params["topic"])
		}

		r.JSON(200, map[string]interface{}{"Queues": topics.TopicMap[params["topic"]].ListQueues()})
	})

	m.Put("/topics/:topic/message", func(r render.Render, params martini.Params, req *http.Request) {

		var present bool
		_, present = topics.TopicMap[params["topic"]]
		if present != true {
			topics.InitTopic(params["topic"])
		}
		var buf bytes.Buffer
		buf.ReadFrom(req.Body)

		response := topics.TopicMap[params["topic"]].Broadcast(cfg, buf.String())
		r.JSON(200, response)
	})

	m.Get("/queues/:queue", func(r render.Render, params martini.Params) {
		//check if we've initialized this queue yet
		var present bool
		_, present = queues.QueueMap[params["queue"]]
		if present != true {
			queues.InitQueue(cfg, params["queue"])
		}
		settings := cfg.GetQueueSettings(params["queue"])
		queueReturn := make(map[string]interface{})
		queueReturn["visibility"] = settings[config.VISIBILITY_TIMEOUT]
		queueReturn["partitions"] = queues.QueueMap[params["queue"]].Parts.PartitionCount()
		r.JSON(200, queueReturn)

	})

	m.Get("/queues/:queue/messages/:batchSize", func(r render.Render, params martini.Params) {
		//check if we've initialized this queue yet
		var present bool
		_, present = queues.QueueMap[params["queue"]]
		if present != true {
			queues.InitQueue(cfg, params["queue"])
		}
		batchSize, err := strconv.ParseUint(params["batchSize"], 10, 32)
		if err != nil {
			//log the error for unparsable input
			log.Println(err)
			r.JSON(422, err.Error())
		}
		messages, err := queues.QueueMap[params["queue"]].Get(cfg, list, uint32(batchSize))
		//TODO move this into the Queue.Get code
		messageList := make([]map[string]interface{}, 0, 10)
		//Format response
		for _, object := range messages {
			message := make(map[string]interface{})
			message["id"] = object.Key
			message["body"] = string(object.Data[:])
			messageList = append(messageList, message)
		}
		if err != nil {
			log.Println(err)
			r.JSON(204, err.Error())
		} else {
			r.JSON(200, messageList)
		}
	})

	m.Put("/queues/:queue/messages", func(params martini.Params, req *http.Request) string {
		var present bool
		_, present = queues.QueueMap[params["queue"]]
		if present != true {
			queues.InitQueue(cfg, params["queue"])
		}

		// parse the request body into a sting
		// TODO clean this up, full json api?
		var buf bytes.Buffer
		buf.ReadFrom(req.Body)
		uuid := queues.QueueMap[params["queue"]].Put(cfg, buf.String())

		return uuid
	})

	m.Delete("/queues/:queue/message/:messageId", func(r render.Render, params martini.Params) {
		var present bool
		_, present = queues.QueueMap[params["queue"]]
		if present != true {
			queues.InitQueue(cfg, params["queue"])
		}

		r.JSON(200, queues.QueueMap[params["queue"]].Delete(cfg, params["messageId"]))
	})

	// DATA INTERACTION API BLOCK

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(cfg.Core.HttpPort), m))
}
