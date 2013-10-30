// Copyright 2012, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package memcache

import (
	"bufio"
	"fmt"
	"io"
	"net"
    "math/rand"
	"strconv"
	"strings"
)

type Connection struct {
	conn     net.Conn
	buffered bufio.ReadWriter
    has_error bool
}

func Connect(addresses ...string) (conn *Connection, err error) {
    if len(addresses) == 0 {
        return nil, fmt.Errorf("Invalid addresses")
    }

    idx := rand.Intn(len(addresses))

    address := addresses[idx]
    fmt.Println("Memcache address is", address)

	var network string
	if strings.Contains(address, "/") {
		network = "unix"
	} else {
		network = "tcp"
	}
	var nc net.Conn
	nc, err = net.Dial(network, address)
	if err != nil {
		return nil, err
	}
	return newConnection(nc), nil
}

func newConnection(nc net.Conn) *Connection {
	return &Connection{
		conn: nc,
		buffered: bufio.ReadWriter{
			bufio.NewReader(nc),
			bufio.NewWriter(nc),
		},
	}
}

func (self *Connection) Close() {
	self.conn.Close()
	self.conn = nil
}

func (self *Connection) IsClosed() bool {
	return self.conn == nil
}

func (self *Connection) HasError() bool {
    return self.has_error
}

func (self *Connection) Get(key string) (value []byte, flags uint16, err error) {
	defer handleError(self, &err)
	value, flags, _ = self.get("get", key)
	return
}

func (self *Connection) Gets(key string) (value []byte, flags uint16, cas uint64, err error) {
	defer handleError(self, &err)
	value, flags, cas = self.get("gets", key)
	return
}

func (self *Connection) Set(key string, flags uint16, timeout uint64, value []byte) (stored bool, err error) {
	defer handleError(self, &err)
	return self.store("set", key, flags, timeout, value, 0), nil
}

func (self *Connection) Add(key string, flags uint16, timeout uint64, value []byte) (stored bool, err error) {
	defer handleError(self, &err)
	return self.store("add", key, flags, timeout, value, 0), nil
}

func (self *Connection) Replace(key string, flags uint16, timeout uint64, value []byte) (stored bool, err error) {
	defer handleError(self, &err)
	return self.store("replace", key, flags, timeout, value, 0), nil
}

func (self *Connection) Append(key string, flags uint16, timeout uint64, value []byte) (stored bool, err error) {
	defer handleError(self, &err)
	return self.store("append", key, flags, timeout, value, 0), nil
}

func (self *Connection) Prepend(key string, flags uint16, timeout uint64, value []byte) (stored bool, err error) {
	defer handleError(self, &err)
	return self.store("prepend", key, flags, timeout, value, 0), nil
}

func (self *Connection) Cas(key string, flags uint16, timeout uint64, value []byte, cas uint64) (stored bool, err error) {
	defer handleError(self, &err)
	return self.store("cas", key, flags, timeout, value, cas), nil
}

func (self *Connection) Version() (version string, err error) {
	defer handleError(self, &err)
    self.writestrings("version\r\n")
    reply := self.readline()
    if strings.Contains(reply, "ERROR") {
        panic(NewMemcacheError("Server error"))
    }
    if !strings.HasPrefix(reply, "VERSION ") {
        panic(NewMemcacheError("%v", reply))
    }
    return strings.Split(reply, " ")[1], nil
}

func (self *Connection) Delete(key string) (deleted bool, err error) {
	defer handleError(self, &err)
	// delete <key> [<time>] [noreply]\r\n
	self.writestrings("delete ", key, "\r\n")
	reply := self.readline()
	if strings.Contains(reply, "ERROR") {
		panic(NewMemcacheError("Server error"))
	}
	return strings.HasPrefix(reply, "DELETED"), nil
}

func (self *Connection) Stats(argument string) (result []byte, err error) {
	defer handleError(self, &err)
	if argument == "" {
		self.writestrings("stats\r\n")
	} else {
		self.writestrings("stats ", argument, "\r\n")
	}
	self.flush()
	for {
		l := self.readline()
		if strings.HasPrefix(l, "END") {
			break
		}
		if strings.Contains(l, "ERROR") {
			return nil, NewMemcacheError(l)
		}
		result = append(result, l...)
		result = append(result, '\n')
	}
	return result, err
}

func (self *Connection) get(command, key string) (value []byte, flags uint16, cas uint64) {
	// get(s) <key>*\r\n
	self.writestrings(command, " ", key, "\r\n")
	header := self.readline()
	if strings.HasPrefix(header, "VALUE") {
		// VALUE <key> <flags> <bytes> [<cas unique>]\r\n
		chunks := strings.Split(header, " ")
		if len(chunks) < 4 {
			panic(NewMemcacheError("Malformed response: %s", string(header)))
		}
		flags64, err := strconv.ParseUint(chunks[2], 10, 16)
		if err != nil {
			panic(NewMemcacheError("%v", err))
		}
		flags = uint16(flags64)
		size, err := strconv.ParseUint(chunks[3], 10, 64)
		if err != nil {
			panic(NewMemcacheError("%v", err))
		}
		if len(chunks) == 5 {
			cas, err = strconv.ParseUint(chunks[4], 10, 64)
			if err != nil {
				panic(NewMemcacheError("%v", err))
			}
		}
		// <data block>\r\n
		value = self.read(int(size) + 2)[:size]
		header = self.readline()
	}
	if !strings.HasPrefix(header, "END") {
		panic(NewMemcacheError("Malformed response: %s", string(header)))
	}
	return value, flags, cas
}

func (self *Connection) store(command, key string, flags uint16, timeout uint64, value []byte, cas uint64) (stored bool) {
	if len(value) > 1000000 {
		return false
	}

	// <command name> <key> <flags> <exptime> <bytes> [noreply]\r\n
	self.writestrings(command, " ", key, " ")
	self.write(strconv.AppendUint(nil, uint64(flags), 10))
	self.writestring(" ")
	self.write(strconv.AppendUint(nil, timeout, 10))
	self.writestring(" ")
	self.write(strconv.AppendInt(nil, int64(len(value)), 10))
	if cas != 0 {
		self.writestring(" ")
		self.write(strconv.AppendUint(nil, cas, 10))
	}
	self.writestring("\r\n")
	// <data block>\r\n
	self.write(value)
	self.writestring("\r\n")
	reply := self.readline()
	if strings.Contains(reply, "ERROR") {
		panic(NewMemcacheError("Server error"))
	}
	return strings.HasPrefix(reply, "STORED")
}

func (self *Connection) writestrings(strs ...string) {
	for _, s := range strs {
		self.writestring(s)
	}
}

func (self *Connection) writestring(s string) {
	if _, err := self.buffered.WriteString(s); err != nil {
		panic(NewMemcacheError("%s", err))
	}
}

func (self *Connection) write(b []byte) {
	if _, err := self.buffered.Write(b); err != nil {
		panic(NewMemcacheError("%s", err))
	}
}

func (self *Connection) flush() {
	if err := self.buffered.Flush(); err != nil {
		panic(NewMemcacheError("%s", err))
	}
}

func (self *Connection) readline() string {
	self.flush()
	l, isPrefix, err := self.buffered.ReadLine()
	if isPrefix || err != nil {
		panic(NewMemcacheError("Prefix: %v, %s", isPrefix, err))
	}
	return string(l)
}

func (self *Connection) read(count int) []byte {
	self.flush()
	b := make([]byte, count)
	if _, err := io.ReadFull(self.buffered, b); err != nil {
		panic(NewMemcacheError("%s", err))
	}
	return b
}

type MemcacheError struct {
	Message string
}

func NewMemcacheError(format string, args ...interface{}) MemcacheError {
	return MemcacheError{fmt.Sprintf(format, args...)}
}

func (self MemcacheError) Error() string {
	return self.Message
}

func handleError(c *Connection, err *error) {
	if x := recover(); x != nil {
		*err = x.(MemcacheError)
        c.has_error = true
	}
}
