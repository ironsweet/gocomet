package gocomet

type MetaMessage struct {
	Channel                  string      `json:"channel"`
	Version                  string      `json:"version"`
	MinimumVersion           string      `json:"minimumVersion"`
	SupportedConnectionTypes []string    `json:"supportedConnectionTypes"`
	ClientId                 string      `json:"clientId"`
	Successful               bool        `json:"successful"`
	AuthSuccessful           bool        `json:"authSuccessful"`
	Advice                   *Advice     `json:"advice"`
	ConnectionType           string      `json:"connectionType"`
	Error                    string      `json:"error"`
	Timestamp                string      `json:"timestamp"` // optional
	Data                     interface{} `json:"data"`
	Id                       string      `json:"id"`
	Subscription             string      `json:"subscription"`
	wait                     bool
}

type Advice struct {
	Reconnect string `json:"reconnect"`
	Timeout   int    `json:"timeout"`
	Interval  int    `json:"interval"`
}

/*
A stand-alone comet server for quick test.
*/
func main() {

}
