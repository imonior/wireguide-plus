package tunnel

import "net"

// This file defines the minimal backend interfaces the engine layer needs
// from either protocol implementation. wireguard-go (zx2c4) and
// amneziawg-go expose identical APIs but are separate packages with
// unrelated types, so the engine holds these interfaces instead of
// importing a concrete device/tun/bind type. Both backends satisfy them.

// wgDevice abstracts the protocol device: the UAPI-config surface of
// wireguard-go's device.Device and amneziawg-go's device.Device.
type wgDevice interface {
	IpcSet(uapiConf string) error
	IpcGet() (string, error)
	IpcHandle(socket net.Conn)
	Up() error
	Close()
}

// tunDevice abstracts the TUN handle used to create the device. Both
// wireguard-go's tun.Device and amneziawg-go's tun.Device satisfy it.
type tunDevice interface {
	Name() (string, error)
	Close() error
}

// bindSocketPinner is the minimal socket-pinning capability shared by the
// conn.Bind implementations of both backends. It exists as a separate
// interface because the Windows socket-pinning code (socketbind_windows.go)
// pins the protocol's UDP socket to the physical underlay after Connect,
// and must work for whichever backend is in use.
type bindSocketPinner interface {
	BindSocketToInterface4(interfaceIndex uint32, blackhole bool) error
	BindSocketToInterface6(interfaceIndex uint32, blackhole bool) error
}
