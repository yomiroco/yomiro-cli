// Package mqtt implements the MQTT broker connector.
package mqtt

import (
	"context"
	"fmt"
	"sync"
	"time"

	mqttlib "github.com/eclipse/paho.mqtt.golang"
	c "github.com/yomiroco/yomiro-cli/internal/gw/connectors"
)

type Handler struct{}

func New() *Handler { return &Handler{} }

func (h *Handler) Inspect(ctx context.Context, host string, port int, t *c.Tracer) (*c.InspectResult, error) {
	if port == 0 {
		port = 1883
	}
	addr := fmt.Sprintf("tcp://%s:%d", host, port)
	start := time.Now()
	opts := mqttlib.NewClientOptions().AddBroker(addr).
		SetClientID("yomiro-gw-inspect").SetConnectTimeout(5 * time.Second)
	client := mqttlib.NewClient(opts)
	tok := client.Connect()
	if !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Record("mqtt_connect", "mosquitto_pub -h "+host, start, "fail")
		return &c.InspectResult{Reachable: false, Host: host, Port: port}, nil
	}
	defer client.Disconnect(100)
	t.Record("mqtt_connect", "mosquitto_pub -h "+host, start, "ok")

	// Read $SYS/# briefly to capture broker version + client count.
	cfg := map[string]any{}
	var mu sync.Mutex
	subStart := time.Now()
	client.Subscribe("$SYS/#", 0, func(_ mqttlib.Client, msg mqttlib.Message) {
		mu.Lock()
		cfg[msg.Topic()] = string(msg.Payload())
		mu.Unlock()
	}).Wait()
	time.Sleep(800 * time.Millisecond)
	t.Record("mqtt_sys_browse", "mosquitto_sub -h "+host+" -t '$SYS/#' -W 1", subStart, fmt.Sprintf("%d topics", len(cfg)))

	mu.Lock()
	defer mu.Unlock()
	version := ""
	if v, ok := cfg["$SYS/broker/version"]; ok {
		version = fmt.Sprintf("%v", v)
	}
	return &c.InspectResult{
		Reachable:     true,
		ServiceType:   "mqtt-broker",
		Version:       version,
		Host:          host,
		Port:          port,
		CurrentConfig: cfg,
		AvailableActions: []c.ActionDefinition{
			{Action: "publish", Description: "Publish a single message", RequiredParams: []string{"topic", "payload"}},
			{Action: "subscribe_test", Description: "Subscribe to a topic and report receipt for N seconds", RequiredParams: []string{"topic"}, OptionalParams: []string{"timeout_s"}},
		},
	}, nil
}

func (h *Handler) Configure(ctx context.Context, host string, port int, action string, cfg map[string]any, dryRun bool, t *c.Tracer) (*c.ConfigureResult, error) {
	if dryRun {
		return &c.ConfigureResult{DryRun: true, Preview: map[string]any{"description": fmt.Sprintf("would %s on %s:%d", action, host, port)}}, nil
	}
	switch action {
	case "publish":
		topic, _ := cfg["topic"].(string)
		payload, _ := cfg["payload"].(string)
		opts := mqttlib.NewClientOptions().AddBroker(fmt.Sprintf("tcp://%s:%d", host, port)).
			SetClientID("yomiro-gw-pub").SetConnectTimeout(5 * time.Second)
		client := mqttlib.NewClient(opts)
		if tok := client.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
			return nil, fmt.Errorf("connect: %v", tok.Error())
		}
		defer client.Disconnect(100)
		if tok := client.Publish(topic, 0, false, payload); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
			return nil, fmt.Errorf("publish: %v", tok.Error())
		}
		return &c.ConfigureResult{DryRun: false, Applied: true, ChangesApplied: []string{"published 1 message to " + topic}}, nil
	default:
		return nil, fmt.Errorf("unknown action %q", action)
	}
}

func (h *Handler) Verify(ctx context.Context, host string, port int, expect map[string]any, t *c.Tracer) (*c.VerifyResult, error) {
	res, err := h.Inspect(ctx, host, port, t)
	if err != nil {
		return nil, err
	}
	return &c.VerifyResult{Healthy: res.Reachable, Checks: []c.HealthCheck{{Check: "broker_reachable", Passed: res.Reachable, Detail: ""}}}, nil
}

func (h *Handler) AvailableActions() []c.ActionDefinition {
	return []c.ActionDefinition{
		{Action: "publish", Description: "Publish a single message"},
		{Action: "subscribe_test", Description: "Subscribe and report receipt"},
	}
}
