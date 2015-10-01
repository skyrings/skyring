package ssh

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
)

func getFingerprint(key ssh.PublicKey) (fingerprint string) {
	md5sum := md5.Sum(key.Marshal())
	for i, c := range hex.EncodeToString(md5sum[:]) {
		if i != 0 && i%2 == 0 {
			fingerprint += ":"
		}
		fingerprint += string(c)
	}
	return
}

func GetFingerprint(host string, port uint) (fingerprint string, err error) {
	sshConfig := &ssh.ClientConfig{
		User: "",
		Auth: []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			fingerprint = getFingerprint(key)
			return nil
		},
	}

	if _, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig); fingerprint != "" {
		// we got fingerprint, no need to return error
		err = nil
	}

	return
}

func Run(command string, host string, port uint, fingerprint string, username string, password string) (stdout string, stderr string, err error) {
	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			finger := getFingerprint(key)
			if finger != fingerprint {
				return errors.New("fingerprint does not match")
			}
			return nil
		},
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig)
	if err != nil {
		return
	}

	session, err := client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()

	var sout, serr bytes.Buffer
	session.Stdout = &sout
	session.Stderr = &serr

	err = session.Run(command)

	stdout = sout.String()
	stderr = serr.String()
	return
}
