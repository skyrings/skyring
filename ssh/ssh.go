package ssh

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/ssh"
	"net"
)

func GetFingerprint(host string, port uint) (string, error) {
	var fingerprint string
	sshConfig := &ssh.ClientConfig{
		User: "",
		Auth: []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			md5sum := md5.Sum(key.Marshal())
			for i, c := range hex.EncodeToString(md5sum[:]) {
				if i != 0 && i%2 == 0 {
					fingerprint += ":"
				}
				fingerprint += string(c)
			}
			return nil
		},
	}

	if _, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig); err != nil {
		if len(fingerprint) == 0 {
			return "", err
		}
	}

	return fingerprint, nil
}
