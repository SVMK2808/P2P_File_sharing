package common

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
)

func Send(conn net.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(data)))
	if _, err = conn.Write(length); err != nil {
		return err
	}

	_, err = conn.Write(data)
	return err
}

func Recv(conn net.Conn, v any) error {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil{
		return err
	}

	n := binary.BigEndian.Uint32(lenBuf)
	data := make([]byte, n)


	if _, err := io.ReadFull(conn, data); err != nil {
		return err
	}

	return json.Unmarshal(data, v)

}