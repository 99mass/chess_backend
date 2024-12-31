package service

import (
    "sync"
    "sync/atomic"
    "time"
)

type InvitationTimeout struct {
    roomID    string
    timer     *time.Timer
    stopChan  chan struct{}
    mutex     sync.RWMutex
    onTimeout func()
    isStopped atomic.Bool
}

func NewInvitationTimeout(roomID string, duration time.Duration, onTimeout func()) *InvitationTimeout {
    it := &InvitationTimeout{
        roomID:    roomID,
        stopChan:  make(chan struct{}),
        onTimeout: onTimeout,
    }
    it.isStopped.Store(false)
    return it
}

func (it *InvitationTimeout) Start() {
    it.mutex.Lock()
    it.timer = time.NewTimer(10 * time.Second)
    it.mutex.Unlock()

    go func() {
        select {
        case <-it.timer.C:
            if !it.isStopped.Load() {
                if it.onTimeout != nil {
                    it.onTimeout()
                }
            }
        case <-it.stopChan:
            return
        }
    }()
}

func (it *InvitationTimeout) Stop() {
    if it.isStopped.Swap(true) {
        return // Déjà arrêté
    }

    it.mutex.Lock()
    defer it.mutex.Unlock()

    if it.timer != nil {
        it.timer.Stop()
    }
    
    select {
    case <-it.stopChan:
        // Canal déjà fermé
    default:
        close(it.stopChan)
    }
}