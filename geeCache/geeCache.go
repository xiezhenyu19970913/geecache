package geeCache

import (
	"fmt"
	"geecache"
	"geecache/singleflight"
	"log"
	"sync"
)

type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Group struct {
	name      string
	getter    Getter
	mainCache geecache.Cache
	peers     geecache.PeerPicker
	loader    *singleflight.Group
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("nil getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: geecache.Cache{CacheBytes: cacheBytes},
		loader: &singleflight.Group{},
	}
	groups[name] = g
	return g
}

func GetGroup(name string) *Group {
	mu.RLock()
	g := groups[name]
	mu.RUnlock()
	return g
}

func (g *Group) Get(key string) (geecache.ByteView, error) {
	if key == "" {
		return geecache.ByteView{}, fmt.Errorf("key is required")
	}
	if v, ok := g.mainCache.Get(key); ok {
		log.Println("get success")
		return v, nil
	}
	return g.load(key)
}

func (g *Group) load(key string) (value geecache.ByteView, err error) {
	viewi,err:=g.loader.Do(key, func() (interface{}, error) {
		if g.peers!=nil{
			if peer,ok:=g.peers.PickPeer(key);ok{
				if value,err=g.getFromPeer(peer,key);err==nil{
					return value,nil
				}
				log.Println("geecache failed to get from peer")
			}
		}
		return g.getLocally(key)
	})
	if err==nil{
		return viewi.(geecache.ByteView),nil
	}
	return
}

func (g *Group) getLocally(key string) (geecache.ByteView, error) {
	bytes, err := g.getter.Get(key)
	if err != nil {
		return geecache.ByteView{}, err
	}
	value := geecache.ByteView{B: geecache.CloneByte(bytes)}
	g.populateCache(key, value)
	return value, nil
}

func (g *Group) populateCache(key string, value geecache.ByteView) {
	g.mainCache.Add(key, value)
}

func (g *Group) RegisterPeers(peers geecache.PeerPicker) {
	if g.peers != nil {
		panic("registerpeerpicker called more than once")
	}
	g.peers = peers
}

func (g Group) getFromPeer(peer geecache.PeerGetter, key string) (geecache.ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return geecache.ByteView{}, err
	}
	return geecache.ByteView{B: bytes}, nil
}
