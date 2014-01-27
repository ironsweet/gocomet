gocomet
=======

A [Go](http://golang.org/) implementation of the [Bayeux Protocol](http://cometd.org/documentation/bayeux/spec).

Installation
------------

```bash
go get github.com/balzaczyy/gocomet
```
    
Usage
-----

```go
import comet "github.com/balzaczyy/gocomet"
...
http.Handle("/cometd", comet.New())
```

License
-------

MIT, see LICENSE.txt.
