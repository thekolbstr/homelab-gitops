package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"cluster-monitor-backend/internal/alerts"
	"cluster-monitor-backend/internal/api"
	"cluster-monitor-backend/internal/push"
	"cluster-monitor-backend/internal/watcher"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	kubeconfig := os.Getenv("KUBECONFIG")

	var config *rest.Config
	var err error
	if kubeconfig != "" {
		// Running outside the cluster (local dev)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		// Running inside the cluster — use the mounted ServiceAccount token
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Fatalf("failed to load kube config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("failed to create k8s clientset: %v", err)
	}

	store := alerts.NewStore()

	// pusher stays a true nil interface (not a typed-nil pointer) if nothing
	// is configured, so the `pusher != nil` checks downstream work correctly.
	var pusher push.AlertNotifier

	switch strings.ToLower(os.Getenv("PUSH_BACKEND")) {
	case "apns":
		apnsSender, err := push.NewAPNsSender(
			os.Getenv("APNS_KEY_PATH"),
			os.Getenv("APNS_KEY_ID"),
			os.Getenv("APNS_TEAM_ID"),
			os.Getenv("APNS_BUNDLE_ID"),
			os.Getenv("APNS_PRODUCTION") == "true",
		)
		if err != nil {
			log.Printf("WARNING: APNs not configured (%v) — push notifications disabled, API still works", err)
		} else {
			pusher = apnsSender
			log.Printf("push backend: APNs")
		}
	default: // "ntfy" or unset — no paid Apple Developer account required
		ntfySender, err := push.NewNtfySender(
			envOrDefault("NTFY_URL", "https://ntfy.sh"),
			os.Getenv("NTFY_TOPIC"),
			os.Getenv("NTFY_TOKEN"),
		)
		if err != nil {
			log.Printf("WARNING: ntfy not configured (%v) — push notifications disabled, API still works", err)
		} else {
			pusher = ntfySender
			log.Printf("push backend: ntfy (topic configured)")
		}
	}

	hub := api.NewHub()
	go hub.Run()

	w := watcher.New(clientset, store, func(evt alerts.Event) {
		store.Add(evt)
		hub.Broadcast(evt)
		if pusher != nil {
			if err := pusher.Notify(evt); err != nil {
				log.Printf("push notify error: %v", err)
			}
		}
	})

	namespace := os.Getenv("WATCH_NAMESPACE") // empty = all namespaces
	go w.Run(namespace)

	server := api.NewServer(store, hub, pusher)
	addr := ":8080"
	log.Printf("cluster-monitor backend listening on %s (watching namespace=%q)", addr, namespace)
	log.Fatal(http.ListenAndServe(addr, server.Router()))
}
