package provisioning

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	baremetalSecretName = "metal3-mariadb-password" // #nosec
	baremetalSecretKey  = "password"
	ironicUsernameKey   = "username"
	ironicPasswordKey   = "password"
	ironicHtpasswdKey   = "htpasswd"
	ironicConfigKey     = "auth-config"
	ironicSecretName    = "metal3-ironic-password"
	ironicUsername      = "ironic-user"
	inspectorSecretName = "metal3-ironic-inspector-password"
	inspectorUsername   = "inspector-user"
)

func generateRandomPassword() (string, error) {
	chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789")
	length := 16
	buf := make([]rune, length)
	numChars := big.NewInt(int64(len(chars)))
	for i := range buf {
		c, err := rand.Int(rand.Reader, numChars)
		if err != nil {
			return "", err
		}
		buf[i] = chars[c.Uint64()]
	}
	return string(buf), nil
}

// CreateMariadbPasswordSecret creates a Secret for Mariadb password
func CreateMariadbPasswordSecret(client coreclientv1.SecretsGetter, targetNamespace string) error {
	_, err := client.Secrets(targetNamespace).Get(context.Background(), baremetalSecretName, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return err
	}

	// Secret does not already exist. So, create one.
	password, err := generateRandomPassword()
	if err != nil {
		return err
	}
	_, err = client.Secrets(targetNamespace).Create(
		context.Background(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      baremetalSecretName,
				Namespace: targetNamespace,
			},
			StringData: map[string]string{
				baremetalSecretKey: password,
			},
		},
		metav1.CreateOptions{},
	)
	return err
}

// CreateIronicPasswordSecret creates a Secret for the Ironic Password
func CreateIronicPasswordSecret(client coreclientv1.SecretsGetter, targetNamespace string) error {
	return createIronicSecret(client, targetNamespace, ironicSecretName, ironicUsername, "ironic")
}

// CreateInspectorPasswordSecret creates a Secret for the Ironic Inspector Password
func CreateInspectorPasswordSecret(client coreclientv1.SecretsGetter, targetNamespace string) error {
	return createIronicSecret(client, targetNamespace, inspectorSecretName, inspectorUsername, "inspector")
}

func createIronicSecret(client coreclientv1.SecretsGetter, targetNamespace string, name string, username string, configSection string) error {
	_, err := client.Secrets(targetNamespace).Get(context.Background(), name, metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		return err
	}

	// Secret does not already exist. So, create one.
	password, err := generateRandomPassword()
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 5) // Use same cost as htpasswd default
	if err != nil {
		return err
	}
	// Change hash version from $2a$ to $2y$, as generated by htpasswd.
	// These are equivalent for our purposes.
	// Some background information about this : https://en.wikipedia.org/wiki/Bcrypt#Versioning_history
	// There was a bug 9 years ago in PHP's implementation of 2a, so they decided to call the fixed version 2y.
	// httpd decided to adopt this (if it sees 2a it uses elaborate heuristic workarounds to mitigate against the bug,
	// but 2y is assumed to not need them), but everyone else (including go) was just decided to not implement the bug in 2a.
	// The bug only affects passwords containing characters with the high bit set, i.e. not ASCII passwords generated here.

	// Anyway, Ironic implemented their own basic auth verification and originally hard-coded 2y because that's what
	// htpasswd produces (see https://review.opendev.org/738718). It is better to keep this as one day we may move the auth
	// to httpd and this would prevent triggering the workarounds.
	hash[2] = 'y'

	_, err = client.Secrets(targetNamespace).Create(
		context.Background(),
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: targetNamespace,
			},
			StringData: map[string]string{
				ironicUsernameKey: username,
				ironicPasswordKey: password,
				ironicHtpasswdKey: fmt.Sprintf("%s:%s", username, hash),
				ironicConfigKey: fmt.Sprintf(`[%s]
auth_type = http_basic
username = %s
password = %s
`,
					configSection, username, password),
			},
		},
		metav1.CreateOptions{},
	)
	return err
}
