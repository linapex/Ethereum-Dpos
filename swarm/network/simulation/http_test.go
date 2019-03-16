
//<developer>
//    <name>linapex 曹一峰</name>
//    <email>linapex@163.com</email>
//    <wx>superexc</wx>
//    <qqgroup>128148617</qqgroup>
//    <url>https://jsq.ink</url>
//    <role>pku engineer</role>
//    <date>2019-03-16 12:09:47</date>
//</624342673821536256>

//
//
//
//
//
//
//
//
//
//
//
//
//
//
//

package simulation

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p/simulations/adapters"
)

func TestSimulationWithHTTPServer(t *testing.T) {
	log.Debug("Init simulation")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	sim := New(
		map[string]ServiceFunc{
			"noop": func(_ *adapters.ServiceContext, b *sync.Map) (node.Service, func(), error) {
				return newNoopService(), nil, nil
			},
		}).WithServer(DefaultHTTPSimAddr)
	defer sim.Close()
	log.Debug("Done.")

	_, err := sim.AddNode()
	if err != nil {
		t.Fatal(err)
	}

	log.Debug("Starting sim round and let it time out...")
//
//
	result := sim.Run(ctx, func(ctx context.Context, sim *Simulation) error {
		log.Debug("Just start the sim without any action and wait for the timeout")
//
		time.Sleep(2 * time.Second)
		return nil
	})

	if result.Error != nil {
		if result.Error.Error() == "context deadline exceeded" {
			log.Debug("Expected timeout error received")
		} else {
			t.Fatal(result.Error)
		}
	}

//
//
	log.Debug("Starting sim round and wait for frontend signal...")
//
	ctx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	go sendRunSignal(t)
	result = sim.Run(ctx, func(ctx context.Context, sim *Simulation) error {
		log.Debug("This run waits for the run signal from `frontend`...")
//
		time.Sleep(2 * time.Second)
		return nil
	})
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	log.Debug("Test terminated successfully")
}

func sendRunSignal(t *testing.T) {
//
	time.Sleep(2 * time.Second)
//

	log.Debug("Sending run signal to simulation: POST /runsim...")
resp, err := http.Post(fmt.Sprintf("http://
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			log.Error("Error closing response body", "err", err)
		}
	}()
	log.Debug("Signal sent")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("err %s", resp.Status)
	}
}

