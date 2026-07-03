package tunnel

type QuickURLSink interface {
	OnQuickTunnelURL(requestID, url string)
}

var quickURLSink QuickURLSink

func SetQuickURLSink(s QuickURLSink) {
	quickURLSink = s
}
