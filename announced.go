package announced

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/influxdb/telegraf/plugins"
)

const (
	//MaxDataGramSize maximum receivable size
	MaxDataGramSize int = 8192
)

//Response announced response
type Response struct {
	Address net.UDPAddr
	Raw     []byte
}

//Collector for type of response
type Collector struct {
	collectType string
	connection  *net.UDPConn   // UDP socket
	queue       chan *Response // received responses
}

//Node response of node
type Node struct {
	time       time.Time
	Statistics interface{} `json:"statistics"`
	Nodeinfo   interface{} `json:"nodeinfo"`
	Neighbours interface{} `json:"neighbours"`
}
type Nodes struct {
	list           map[string]*Node
	sync.Mutex
}
//Announced the service
type Announced struct {
	Interface       string
	Destination     string
	Collector       []string
	Port            int
	RequestInterval int64 `toml:"request_interval"`
	nodes           *Nodes
	time            time.Time
}

var sampleConfig = `
  # a name for the service being polled
  interface = "eth0" # not implemented
  collector = ['statistics','nodeinfo','neighbours'] # not implemented
  destination = "ff02:0:0:0:0:0:2:1001"
  port = 1001
  request_interval = 5000 #in millisecond
`

//SampleConfig of Service
func (h *Announced) SampleConfig() string {
	return sampleConfig
}

//Description of Service
func (h *Announced) Description() string {
	return "Announced is a Gluon (OpenWRT) service, get fetch information of router. (often used by Freifunk Communities)"
}

func newCollector(collectType string, h *Announced) *Collector {
	// Parse address
	addr, err := net.ResolveUDPAddr("udp", "[::]:0")
	if err != nil {
		log.Panic(err)
	}

	// Open socket
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Panic(err)
	}
	conn.SetReadBuffer(MaxDataGramSize)

	collector := &Collector{
		collectType: collectType,
		connection:  conn,
		queue:       make(chan *Response, 100),
	}

	go collector.sender(h)

	go collector.receiver()
	go collector.parser(h)

	return collector
}
func (coll *Collector) close() {
	coll.connection.Close()
	close(coll.queue)
}

func (coll *Collector) sender(h *Announced) {
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("[%s]:%d", h.Destination, h.Port))

	c := time.Tick(time.Duration(h.RequestInterval) * time.Millisecond)

	for range c {
		coll.connection.WriteToUDP([]byte(coll.collectType), addr)
	}
}

func (coll *Collector) parser(h *Announced) {
	for obj := range coll.queue {
		coll.parse(obj, h)
	}
}
func (coll *Collector) parse(res *Response, h *Announced) {
	var result map[string]interface{}
	json.Unmarshal(res.Raw, &result)

	nodeID, _ := result["node_id"].(string)

	if nodeID == "" {
		log.Println("unable to parse node_id")
		return
	}

	h.nodes.Lock()
	defer h.nodes.Unlock()
	node := h.nodes.list[nodeID]
	if node == nil {
		node = &Node{
			time: time.Now(),
		}
		h.nodes.list[nodeID] = node
	} else {
		node.time = time.Now()
	}

	// Set result
	elem := reflect.ValueOf(node).Elem()
	field := elem.FieldByName(strings.Title(coll.collectType))
	field.Set(reflect.ValueOf(result))
}

func (coll *Collector) receiver() {
	buf := make([]byte, MaxDataGramSize)
	for {
		n, src, err := coll.connection.ReadFromUDP(buf)

		if err != nil {
			log.Println("ReadFromUDP failed:", err)
			return
		}

		raw := make([]byte, n)
		copy(raw, buf)

		coll.queue <- &Response{
			Address: *src,
			Raw:     raw,
		}
	}
}

//Gather of the service
func (h *Announced) Gather(acc plugins.Accumulator) error {
	i := 0
	h.nodes.Lock()
	for nodeID, data := range h.nodes.list {
		if h.time.Before(data.time) {
			tags := map[string]string{"node": nodeID}
			processResponse(acc, "statistics", tags, data.Statistics)
			processResponse(acc, "nodeinfo", tags, data.Nodeinfo)
			processResponse(acc, "neighbours", tags, data.Neighbours)
			i++
		}
	}
	h.nodes.Unlock()
	h.time = time.Now()
	log.Println("plugin-announced", "fetched nodes: ", i)
	return nil
}

func processResponse(acc plugins.Accumulator, prefix string, tags map[string]string, v interface{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, v := range t {
			processResponse(acc, prefix+"_"+k, tags, v)
		}
	case float64:
		acc.Add(prefix, v, tags)
	}
}

func init() {
	plugins.Add("announced", func() plugins.Plugin {
		h := &Announced{nodes:&Nodes{list:make(map[string]*Node)}}
		newCollector("nodeinfo", h)
		newCollector("statistics", h)
		newCollector("neighbours", h)
		return h
	})
}
