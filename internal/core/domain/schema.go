package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
	core "github.com/iden3/go-iden3-core/v2"
	"github.com/iden3/go-iden3-core/v2/w3c"
)

//nolint:gosec //reason: constant
const (
	AuthBJJCredential              = "AuthBJJCredential"
	AuthBJJCredentialJSONSchemaURL = "https://schema.iden3.io/core/json/auth.json"
	AuthBJJCredentialSchemaJSON    = `{"$schema":"http://json-schema.org/draft-07/schema#","$metadata":{"uris":{"jsonLdContext":"https://schema.iden3.io/core/jsonld/auth.jsonld","jsonSchema":"https://schema.iden3.io/core/json/auth.json"},"serialization":{"indexDataSlotA":"x","indexDataSlotB":"y"}},"type":"object","required":["@context","id","type","issuanceDate","credentialSubject","credentialSchema","credentialStatus","issuer"],"properties":{"@context":{"type":["string","array","object"]},"id":{"type":"string"},"type":{"type":["string","array"],"items":{"type":"string"}},"issuer":{"type":["string","object"],"format":"uri","required":["id"],"properties":{"id":{"type":"string","format":"uri"}}},"issuanceDate":{"type":"string","format":"date-time"},"expirationDate":{"type":"string","format":"date-time"},"credentialSchema":{"type":"object","required":["id","type"],"properties":{"id":{"type":"string","format":"uri"},"type":{"type":"string"}}},"credentialSubject":{"type":"object","required":["x","y"],"properties":{"id":{"title":"Credential Subject ID","type":"string","format":"uri"},"x":{"type":"string"},"y":{"type":"string"}}}}}`
	AuthBJJCredentialSchemaType    = "https://schema.iden3.io/core/jsonld/auth.jsonld#AuthBJJCredential"

	AuthBJJCredentialJSONLDContext = "https://schema.iden3.io/core/jsonld/auth.jsonld"
	AuthBJJCredentialTypeID        = "https://schema.iden3.io/core/jsonld/auth.jsonld#AuthBJJCredential"

	Iden3PaymentRailsRequestV1SchemaJSON      = `{"EIP712Domain":[{"name":"name","type":"string"},{"name":"version","type":"string"},{"name":"chainId","type":"uint256"},{"name":"verifyingContract","type":"address"}],"Iden3PaymentRailsRequestV1":[{"name":"recipient","type":"address"},{"name":"amount","type":"uint256"},{"name":"expirationDate","type":"uint256"},{"name":"nonce","type":"uint256"},{"name":"metadata","type":"bytes"}]}`
	Iden3PaymentRailsERC20RequestV1SchemaJSON = `{"EIP712Domain":[{"name":"name","type":"string"},{"name":"version","type":"string"},{"name":"chainId","type":"uint256"},{"name":"verifyingContract","type":"address"}],"Iden3PaymentRailsERC20RequestV1":[{"name":"tokenAddress","type":"address"},{"name":"recipient","type":"address"},{"name":"amount","type":"uint256"},{"name":"expirationDate","type":"uint256"},{"name":"nonce","type":"uint256"},{"name":"metadata","type":"bytes"}]}`
)

// SchemaFormat type
type SchemaFormat string

// SchemaWords is a collection of schema attributes
type SchemaWords []string

// String satisfies the Stringer interface for SchemaWords
func (a SchemaWords) String() string {
	if len(a) == 0 {
		return ""
	}
	return strings.Join(a, ", ")
}

// SchemaWordsFromString is an SchemaWords constructor from a string with  comma separated attributes
func SchemaWordsFromString(commaAttrs string) SchemaWords {
	attrs := strings.Split(commaAttrs, ",")
	schemaAttrs := make(SchemaWords, len(attrs))
	for i, attr := range attrs {
		w := strings.TrimSpace(attr)
		if w != "" {
			schemaAttrs[i] = w
		}
	}
	return schemaAttrs
}

// Schema defines a domain.Schema entity
type Schema struct {
	ID              uuid.UUID
	IssuerDID       w3c.DID
	URL             string
	Type            string
	ContextURL      string
	Title           *string
	Description     *string
	Version         string
	Hash            core.SchemaHash
	Words           SchemaWords
	DisplayMethodID *uuid.UUID
	CreatedAt       time.Time
}
