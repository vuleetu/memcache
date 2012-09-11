fork from vitess:

    http://code.google.com/p/vitess/source/browse/go/memcache/memcache.go

install:

    $ go install github.com/smallfish/memcache

example:

    package main

    import (
        "log"
        "github.com/smallfish/memcache"
    )

    func main() {
        client, err := memcache.Connect("127.0.0.1:11211")
        if err != nil {
            log.Fatalln("error:", err)
        }
        defer client.Close()

        if ok, err := client.Set("name", 0, 0, []byte("smallfish")); !ok {
            log.Println("error:", err)
        }

        val, flag, err := client.Get("name")
        if val != nil {
            log.Println("name:", string(val), "flag:", flag)
        }
    }


__END__
