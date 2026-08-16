package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPConfig struct {
	Host          string
	Port          int
	Username      string
	Password      string
	BaseDirectory string
	HostKeySHA256 string
}

func validateSFTPConfiguration(configuration map[string]any, codePrefix string) error {
	if resolveInputSourceType(configuration) != "SFTP" {
		return nil
	}
	return validateSFTPConnectionConfiguration(configuration, codePrefix)
}

func validateSFTPConnectionConfiguration(configuration map[string]any, codePrefix string) error {
	if configString(configuration, "sftpHost") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_HOST_REQUIRED", Message: "configuration.sftpHost is required"}
	}
	port, ok := configInt(configuration, "sftpPort")
	if !ok || port < 1 || port > 65535 {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_PORT_INVALID", Message: "configuration.sftpPort must be an integer between 1 and 65535"}
	}
	if configString(configuration, "sftpUsername") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_USERNAME_REQUIRED", Message: "configuration.sftpUsername is required"}
	}
	if configString(configuration, "sftpBaseDirectory") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_BASE_DIRECTORY_REQUIRED", Message: "configuration.sftpBaseDirectory is required"}
	}
	if configString(configuration, "sftpHostKeySha256") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_HOST_KEY_REQUIRED", Message: "configuration.sftpHostKeySha256 is required"}
	}
	if configString(configuration, "sftpPasswordEncrypted") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_PASSWORD_REQUIRED", Message: "configuration.sftpPasswordEncrypted is required"}
	}
	return nil
}

func readSFTPInputContent(
	ctx context.Context,
	sftpConfig SFTPConfig,
	fileName string,
	codePrefix string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSFTPRuntimeConfig(sftpConfig); err != nil {
		return nil, &NodeError{
			Category: "INTERNAL",
			Code:     "INPUT_SFTP_NOT_CONFIGURED",
			Message:  "SFTP input is not configured for this runtime.",
		}
	}
	remotePath, err := resolveSFTPRemotePath(sftpConfig.BaseDirectory, fileName)
	if err != nil {
		return nil, &NodeError{
			Category: "INTERNAL",
			Code:     codePrefix + "_READ_FAILED",
			Message:  "The SFTP input file could not be resolved.",
		}
	}

	sftpClient, closeClient, err := openSFTPClient(ctx, sftpConfig)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		message := "The SFTP input source could not be reached."
		if errors.Is(err, errSFTPClientOpen) {
			message = "The SFTP input source could not be opened."
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_SFTP_CONNECT_FAILED",
			Message:  message,
			CanRetry: true,
			Cause:    err,
		}
	}
	defer closeClient()

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_READ_FAILED",
			Message:  "The SFTP input file could not be read.",
			Cause:    err,
		}
	}
	defer remoteFile.Close()

	content, err := io.ReadAll(remoteFile)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_READ_FAILED",
			Message:  "The SFTP input file could not be read.",
			Cause:    err,
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return content, nil
}

func resolveSFTPConfig(
	secrets SecretDecryptor,
	configuration map[string]any,
	codePrefix string,
) (SFTPConfig, error) {
	port, _ := configInt(configuration, "sftpPort")
	resolved := SFTPConfig{
		Host:          configString(configuration, "sftpHost"),
		Port:          port,
		Username:      configString(configuration, "sftpUsername"),
		BaseDirectory: configString(configuration, "sftpBaseDirectory"),
		HostKeySHA256: configString(configuration, "sftpHostKeySha256"),
	}
	encrypted := configString(configuration, "sftpPasswordEncrypted")
	if encrypted != "" {
		if secrets == nil || !secrets.Configured() {
			return SFTPConfig{}, &NodeError{
				Category: "INTERNAL",
				Code:     codePrefix + "_SFTP_SECRET_UNAVAILABLE",
				Message:  "SFTP password decryption is not configured for this runtime.",
			}
		}
		password, err := secrets.Decrypt(encrypted)
		if err != nil {
			return SFTPConfig{}, &NodeError{
				Category: "INTERNAL",
				Code:     codePrefix + "_SFTP_SECRET_DECRYPT_FAILED",
				Message:  "The SFTP password could not be decrypted.",
			}
		}
		resolved.Password = password
	}
	return resolved, nil
}

func validateSFTPRuntimeConfig(sftpConfig SFTPConfig) error {
	if strings.TrimSpace(sftpConfig.Host) == "" ||
		sftpConfig.Port < 1 || sftpConfig.Port > 65535 ||
		strings.TrimSpace(sftpConfig.Username) == "" ||
		sftpConfig.Password == "" ||
		strings.TrimSpace(sftpConfig.BaseDirectory) == "" ||
		strings.TrimSpace(sftpConfig.HostKeySHA256) == "" {
		return errSFTPNotConfigured
	}
	return nil
}

var (
	errSFTPNotConfigured   = errors.New("SFTP is not configured")
	errSFTPClientOpen      = errors.New("SFTP client failed to open")
	errSFTPNoSecureHostKey = errors.New("no secure SFTP host key algorithm is available")
	errSFTPHostKeyMismatch = errors.New("SFTP host key fingerprint mismatch")
)

const (
	sftpIncompatibilityHostKeyFingerprint = "HOST_KEY_FINGERPRINT_MISMATCH"
	sftpIncompatibilityHostKeyAlgorithm   = "HOST_KEY_ALGORITHM_MISMATCH"
)

type sftpHostKeyMismatchError struct {
	hostKeyType string
}

func (mismatch *sftpHostKeyMismatchError) Error() string {
	return errSFTPHostKeyMismatch.Error()
}

func (mismatch *sftpHostKeyMismatchError) Unwrap() error {
	return errSFTPHostKeyMismatch
}

func SFTPIncompatibilityCode(err error) (string, bool) {
	if errors.Is(err, errSFTPHostKeyMismatch) {
		return sftpIncompatibilityHostKeyFingerprint, true
	}
	var negotiationError *ssh.AlgorithmNegotiationError
	if errors.As(err, &negotiationError) && negotiationError.What == "host key" {
		return sftpIncompatibilityHostKeyAlgorithm, true
	}
	return "", false
}

func resolveSFTPRemotePath(baseDirectory, fileName string) (string, error) {
	if !isSafePathSegment(fileName) {
		return "", fmt.Errorf("invalid SFTP file name")
	}
	base := strings.TrimRight(strings.TrimSpace(baseDirectory), "/")
	if base == "" {
		return "", fmt.Errorf("invalid SFTP base directory")
	}
	remotePath := path.Join(base, fileName)
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}
	if path.Base(remotePath) != fileName {
		return "", fmt.Errorf("SFTP target escapes configured base directory")
	}
	return remotePath, nil
}

func openSFTPClient(ctx context.Context, sftpConfig SFTPConfig) (*sftp.Client, func(), error) {
	hostKeyAlgorithms := secureSFTPHostKeyAlgorithms()
	if len(hostKeyAlgorithms) == 0 {
		return nil, nil, errSFTPNoSecureHostKey
	}
	sftpClient, closeClient, err := openSFTPClientAttempt(
		ctx, sftpConfig, hostKeyAlgorithms,
	)
	if err == nil {
		return sftpClient, closeClient, nil
	}
	if !errors.Is(err, errSFTPHostKeyMismatch) {
		return nil, nil, err
	}
	initialHostKeyError := err
	rejectedHostKeyTypes := make(map[string]struct{})
	recordRejectedSFTPHostKeyType(rejectedHostKeyTypes, err)
	for _, hostKeyAlgorithm := range hostKeyAlgorithms {
		if _, rejected := rejectedHostKeyTypes[sftpHostKeyType(hostKeyAlgorithm)]; rejected {
			continue
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, nil, contextErr
		}
		sftpClient, closeClient, err = openSFTPClientAttempt(
			ctx, sftpConfig, []string{hostKeyAlgorithm},
		)
		if err == nil {
			return sftpClient, closeClient, nil
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, nil, contextErr
		}
		if !sftpHostKeyCandidateUnavailable(err) {
			return nil, nil, err
		}
		recordRejectedSFTPHostKeyType(rejectedHostKeyTypes, err)
	}
	return nil, nil, initialHostKeyError
}

func secureSFTPHostKeyAlgorithms() []string {
	supported := make(map[string]struct{})
	for _, algorithm := range ssh.SupportedAlgorithms().HostKeys {
		supported[algorithm] = struct{}{}
	}
	preferred := []string{
		ssh.KeyAlgoECDSA256,
		ssh.KeyAlgoECDSA384,
		ssh.KeyAlgoECDSA521,
		ssh.KeyAlgoED25519,
		ssh.KeyAlgoRSASHA512,
		ssh.KeyAlgoRSASHA256,
	}
	candidates := make([]string, 0, len(preferred))
	seen := make(map[string]struct{}, len(preferred))
	for _, algorithm := range preferred {
		if _, ok := supported[algorithm]; !ok {
			continue
		}
		if _, duplicate := seen[algorithm]; duplicate {
			continue
		}
		seen[algorithm] = struct{}{}
		candidates = append(candidates, algorithm)
	}
	return candidates
}

func recordRejectedSFTPHostKeyType(rejected map[string]struct{}, err error) {
	var mismatch *sftpHostKeyMismatchError
	if errors.As(err, &mismatch) && mismatch.hostKeyType != "" {
		rejected[mismatch.hostKeyType] = struct{}{}
	}
}

func sftpHostKeyType(algorithm string) string {
	switch algorithm {
	case ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256:
		return ssh.KeyAlgoRSA
	default:
		return algorithm
	}
}

func sftpHostKeyCandidateUnavailable(err error) bool {
	if errors.Is(err, errSFTPHostKeyMismatch) {
		return true
	}
	var negotiationError *ssh.AlgorithmNegotiationError
	return errors.As(err, &negotiationError) && negotiationError.What == "host key"
}

func openSFTPClientAttempt(
	ctx context.Context,
	sftpConfig SFTPConfig,
	hostKeyAlgorithms []string,
) (*sftp.Client, func(), error) {
	address := net.JoinHostPort(sftpConfig.Host, fmt.Sprintf("%d", sftpConfig.Port))
	tcpConnection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, nil, contextErr
		}
		return nil, nil, err
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = tcpConnection.Close()
	})

	clientConfig := &ssh.ClientConfig{
		User: sftpConfig.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(sftpConfig.Password),
		},
		HostKeyCallback: fixedHostKeySHA256(sftpConfig.HostKeySHA256),
	}
	if len(hostKeyAlgorithms) > 0 {
		clientConfig.HostKeyAlgorithms = hostKeyAlgorithms
	}
	sshConnection, channels, requests, err := ssh.NewClientConn(tcpConnection, address, clientConfig)
	if err != nil {
		stopCancellation()
		_ = tcpConnection.Close()
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, nil, contextErr
		}
		return nil, nil, err
	}
	sshClient := ssh.NewClient(sshConnection, channels, requests)
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		stopCancellation()
		_ = sshClient.Close()
		_ = tcpConnection.Close()
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, nil, contextErr
		}
		return nil, nil, errors.Join(errSFTPClientOpen, err)
	}
	closeClient := func() {
		stopCancellation()
		_ = sftpClient.Close()
		_ = sshClient.Close()
		_ = tcpConnection.Close()
	}
	return sftpClient, closeClient, nil
}

func fixedHostKeySHA256(expectedFingerprint string) ssh.HostKeyCallback {
	expected := strings.TrimPrefix(
		strings.TrimSpace(expectedFingerprint),
		"SHA256:",
	)
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		actual := strings.TrimPrefix(
			strings.TrimSpace(ssh.FingerprintSHA256(key)),
			"SHA256:",
		)
		if actual != expected {
			return &sftpHostKeyMismatchError{hostKeyType: key.Type()}
		}
		return nil
	}
}
