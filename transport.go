package gocomet

/*
From [The Bayeux Protocol](http://cometd.org/documentation/bayeux/spec):

"Long-polling" is a poolling transport that attempts to minimize both
latency in sever-client message delivery, and the processing/network
resources required for the cnnection.
*/
type LongPolling struct {
}
