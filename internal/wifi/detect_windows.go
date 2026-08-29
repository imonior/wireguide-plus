//go:build windows

package wifi

import (
	"log/slog"
	"sync"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startWindowsWlanWatcher subscribes to Wlanapi connect/disconnect
// notifications and invokes onChange on every transition. Returns a stop
// function. Falls back to no-op (returns nil stop) when wlanapi.dll is
// unavailable (server SKUs without WLAN service, headless containers).
func startWindowsWlanWatcher(onChange func()) (stop func(), attached bool) {
	noop := func() {}

	if err := wlanLazyOpenHandle(); err != nil {
		slog.Debug("wifi: wlanapi.dll OpenHandle failed", "error", err)
		return noop, false
	}

	cb := syscall.NewCallback(func(notif uintptr, _ uintptr) uintptr {
		// We don't filter on notification source/code because the cost
		// of running CurrentSSID() and comparing in monitor.checkNow is
		// trivial. Any wlan event = "re-check SSID".
		onChange()
		return 0
	})

	var prevSource uint32
	ret, _, _ := procWlanRegisterNotification.Call(
		uintptr(wlanHandle),
		uintptr(wlanNotificationSourceACM),
		1, // ignoreDuplicate=TRUE
		cb,
		0, // CallerContext
		0, // Reserved
		uintptr(unsafe.Pointer(&prevSource)),
	)
	if ret != 0 {
		slog.Debug("wifi: WlanRegisterNotification failed", "status", ret)
		return noop, false
	}
	slog.Info("wifi: Wlanapi notification subscribed")

	var once sync.Once
	return func() {
		once.Do(func() {
			// Unregister by passing source=0.
			var prev uint32
			procWlanRegisterNotification.Call(
				uintptr(wlanHandle),
				0,
				0,
				0,
				0,
				0,
				uintptr(unsafe.Pointer(&prev)),
			)
		})
	}, true
}

var (
	modWlanapi                      = windows.NewLazySystemDLL("wlanapi.dll")
	procWlanOpenHandle              = modWlanapi.NewProc("WlanOpenHandle")
	procWlanRegisterNotification    = modWlanapi.NewProc("WlanRegisterNotification")
	procWlanEnumInterfaces          = modWlanapi.NewProc("WlanEnumInterfaces")
	procWlanQueryInterface          = modWlanapi.NewProc("WlanQueryInterface")
	procWlanGetProfileList          = modWlanapi.NewProc("WlanGetProfileList")
	procWlanFreeMemory              = modWlanapi.NewProc("WlanFreeMemory")

	wlanHandle uintptr
	wlanOpenOnce sync.Once
	wlanOpenErr  error
)

const (
	// WLAN_NOTIFICATION_SOURCE_ACM = 0x00000008 covers connect/disconnect/scan.
	wlanNotificationSourceACM = 0x00000008
)

func wlanLazyOpenHandle() error {
	wlanOpenOnce.Do(func() {
		// WlanOpenHandle wants: DWORD dwClientVersion, PVOID pReserved,
		//                       PDWORD pdwNegotiatedVersion, PHANDLE phClientHandle
		var negotiated uint32
		var handle uintptr
		ret, _, _ := procWlanOpenHandle.Call(
			2, // client version 2 (Vista+)
			0,
			uintptr(unsafe.Pointer(&negotiated)),
			uintptr(unsafe.Pointer(&handle)),
		)
		if ret != 0 {
			wlanOpenErr = syscall.Errno(ret)
			return
		}
		wlanHandle = handle
	})
	return wlanOpenErr
}

// ---------------------------------------------------------------------------
// Native SSID reads via Wlanapi. SSIDs come back as raw bytes / UTF-16
// instead of netsh's code-page-dependent console text, so non-ASCII SSIDs
// survive regardless of the system's OEM code page.
// ---------------------------------------------------------------------------

// dot11Ssid mirrors the DOT11_SSID struct.
type dot11Ssid struct {
	ssidLen uint32
	ssid    [32]byte
}

// wlanConnectionAttributes mirrors the WLAN_CONNECTION_ATTRIBUTES struct
// (the leading fields we need; the trailing association attributes are
// intentionally omitted).
type wlanConnectionAttributes struct {
	isState            uint32 // WLAN_INTERFACE_STATE
	wlanConnectionMode uint32
	profileName        [256]uint16
	ssid               dot11Ssid
}

// wlanInterfaceInfoList mirrors WLAN_INTERFACE_INFO_LIST.
type wlanInterfaceInfoList struct {
	numberOfItems uint32
	index         uint32
	interfaceInfo [1]wlanInterfaceInfo
}

// wlanInterfaceInfo mirrors WLAN_INTERFACE_INFO.
type wlanInterfaceInfo struct {
	interfaceGuid        windows.GUID
	interfaceDescription [256]uint16
	state                uint32 // WLAN_INTERFACE_STATE
}

// wlanProfileInfoList mirrors WLAN_PROFILE_INFO_LIST.
type wlanProfileInfoList struct {
	numberOfItems uint32
	index         uint32
	profileInfo   [1]wlanProfileInfo
}

// wlanProfileInfo mirrors WLAN_PROFILE_INFO.
type wlanProfileInfo struct {
	profileName [256]uint16
	flags       uint32
}

const (
	// WLAN_INTERFACE_STATE_CONNECTED = 1
	wlanInterfaceStateConnected = 1
	// WLAN_INTF_OPCODE_CURRENT_CONNECTION = 7
	wlanIntfOpcodeCurrentConnection = 7
)

// currentSSIDFromWlanapi returns the SSID of the currently connected
// interface via WlanQueryInterface, or "" when not connected / API missing.
func currentSSIDFromWlanapi() string {
	if err := wlanLazyOpenHandle(); err != nil {
		return ""
	}
	var listPtr unsafe.Pointer
	ret, _, _ := procWlanEnumInterfaces.Call(uintptr(wlanHandle), 0, uintptr(unsafe.Pointer(&listPtr)))
	if ret != 0 || listPtr == nil {
		return ""
	}
	defer procWlanFreeMemory.Call(uintptr(listPtr))

	list := (*wlanInterfaceInfoList)(listPtr)
	// The trailing interfaceInfo field is declared as [1]wlanInterfaceInfo
	// but is actually a C-style variable-length array. Indexing it with a
	// Go index (list.interfaceInfo[i]) panics the runtime once the count
	// exceeds 1 (multiple adapters), so walk it with pointer arithmetic
	// instead. Same pattern in knownSSIDsFromWlanapi below.
	itemsBase := unsafe.Add(listPtr, unsafe.Offsetof(wlanInterfaceInfoList{}.interfaceInfo))
	itemSize := unsafe.Sizeof(wlanInterfaceInfo{})
	for i := uint32(0); i < list.numberOfItems; i++ {
		info := (*wlanInterfaceInfo)(unsafe.Add(itemsBase, uintptr(i)*itemSize))
		if info.state != wlanInterfaceStateConnected {
			continue
		}
		var dataSize uint32
		var dataPtr unsafe.Pointer
		ret, _, _ := procWlanQueryInterface.Call(
			uintptr(wlanHandle),
			uintptr(unsafe.Pointer(&info.interfaceGuid)),
			uintptr(wlanIntfOpcodeCurrentConnection),
			0, // pReserved
			uintptr(unsafe.Pointer(&dataSize)),
			uintptr(unsafe.Pointer(&dataPtr)),
			0, // pWlanOpcodeValueType
		)
		if ret != 0 || dataPtr == nil {
			continue
		}
		attrs := (*wlanConnectionAttributes)(dataPtr)
		n := attrs.ssid.ssidLen
		if n > 32 {
			n = 32
		}
		ssid := string(attrs.ssid.ssid[:n])
		// Most APs encode the SSID as UTF-8; a few legacy ones use the OEM
		// code page. Try UTF-8 first, fall back to OEM decoding.
		if !utf8.ValidString(ssid) {
			ssid = decodeOEM(attrs.ssid.ssid[:n])
		}
		procWlanFreeMemory.Call(uintptr(dataPtr))
		if ssid != "" {
			return ssid
		}
	}
	return ""
}

// knownSSIDsFromWlanapi lists every saved Wi-Fi profile (SSID) via
// WlanGetProfileList. Profile names are UTF-16 in wlanapi, so non-ASCII
// names come back correctly without any code-page conversion.
func knownSSIDsFromWlanapi() []string {
	if err := wlanLazyOpenHandle(); err != nil {
		return nil
	}
	var listPtr unsafe.Pointer
	ret, _, _ := procWlanEnumInterfaces.Call(uintptr(wlanHandle), 0, uintptr(unsafe.Pointer(&listPtr)))
	if ret != 0 || listPtr == nil {
		return nil
	}
	defer procWlanFreeMemory.Call(uintptr(listPtr))

	seen := make(map[string]bool)
	var result []string
	list := (*wlanInterfaceInfoList)(listPtr)
	itemsBase := unsafe.Add(listPtr, unsafe.Offsetof(wlanInterfaceInfoList{}.interfaceInfo))
	itemSize := unsafe.Sizeof(wlanInterfaceInfo{})
	for i := uint32(0); i < list.numberOfItems; i++ {
		info := (*wlanInterfaceInfo)(unsafe.Add(itemsBase, uintptr(i)*itemSize))
		var profileListPtr unsafe.Pointer
		ret, _, _ := procWlanGetProfileList.Call(
			uintptr(wlanHandle),
			uintptr(unsafe.Pointer(&info.interfaceGuid)),
			0, // pReserved
			uintptr(unsafe.Pointer(&profileListPtr)),
		)
		if ret != 0 || profileListPtr == nil {
			continue
		}
		plist := (*wlanProfileInfoList)(profileListPtr)
		// Same variable-length-array caveat: walk profiles with pointer
		// arithmetic rather than Go indexing.
		profilesBase := unsafe.Add(profileListPtr, unsafe.Offsetof(wlanProfileInfoList{}.profileInfo))
		profileSize := unsafe.Sizeof(wlanProfileInfo{})
		for j := uint32(0); j < plist.numberOfItems; j++ {
			p := (*wlanProfileInfo)(unsafe.Add(profilesBase, uintptr(j)*profileSize))
			name := windows.UTF16ToString(p.profileName[:])
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			result = append(result, name)
		}
		procWlanFreeMemory.Call(uintptr(profileListPtr))
	}
	return result
}
