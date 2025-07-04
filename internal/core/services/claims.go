package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	core "github.com/iden3/go-iden3-core/v2"
	"github.com/iden3/go-iden3-core/v2/w3c"
	"github.com/iden3/go-merkletree-sql/v2"
	"github.com/iden3/go-schema-processor/v2/merklize"
	"github.com/iden3/go-schema-processor/v2/processor"
	"github.com/iden3/go-schema-processor/v2/verifiable"
	"github.com/iden3/iden3comm/v2"
	"github.com/iden3/iden3comm/v2/packers"
	"github.com/iden3/iden3comm/v2/protocol"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/jackc/pgx/v4"

	"github.com/polygonid/sh-id-platform/internal/common"
	"github.com/polygonid/sh-id-platform/internal/config"
	"github.com/polygonid/sh-id-platform/internal/core/domain"
	"github.com/polygonid/sh-id-platform/internal/core/event"
	"github.com/polygonid/sh-id-platform/internal/core/ports"
	"github.com/polygonid/sh-id-platform/internal/db"
	"github.com/polygonid/sh-id-platform/internal/jsonschema"
	"github.com/polygonid/sh-id-platform/internal/loader"
	"github.com/polygonid/sh-id-platform/internal/log"
	"github.com/polygonid/sh-id-platform/internal/pubsub"
	"github.com/polygonid/sh-id-platform/internal/qrlink"
	"github.com/polygonid/sh-id-platform/internal/repositories"
	"github.com/polygonid/sh-id-platform/internal/revocationstatus"
	schemaPkg "github.com/polygonid/sh-id-platform/internal/schema"
	"github.com/polygonid/sh-id-platform/internal/urn"
	"github.com/polygonid/sh-id-platform/internal/utils"
)

var (
	ErrCredentialNotFound                = errors.New("credential not found")                                          // ErrCredentialNotFound Cannot retrieve the given claim
	ErrDisplayMethodLacksURL             = errors.New("credential request with display method lacks url")              // ErrDisplayMethodLacksURL means the credential request includes a display method, but the url is not set
	ErrEmptyMTPProof                     = errors.New("mtp credentials must have a mtp proof to be fetched")           // ErrEmptyMTPProof means that a credential of MTP type can not be fetched if it does not contain the proof
	ErrJSONLdContext                     = errors.New("jsonLdContext must be a string")                                // ErrJSONLdContext Field jsonLdContext must be a string
	ErrInvalidCredentialSubject          = errors.New("credential subject does not match the provided schema")         // ErrInvalidCredentialSubject means the credentialSubject does not match the schema provided
	ErrLinkNotFound                      = errors.New("link not found")                                                // ErrLinkNotFound Cannot get the given link from the DB
	ErrLoadingSchema                     = errors.New("cannot load schema")                                            // ErrLoadingSchema means the system cannot load the schema file
	ErrMalformedURL                      = errors.New("malformed url")                                                 // ErrMalformedURL The schema url is wrong
	ErrParseClaim                        = errors.New("cannot parse claim")                                            // ErrParseClaim Cannot parse claim
	ErrProcessSchema                     = errors.New("cannot process schema")                                         // ErrProcessSchema Cannot process schema
	ErrRefreshServiceLacksExpirationTime = errors.New("credential request with refresh service lacks expiration time") // ErrRefreshServiceLacksExpirationTime means the credential request includes a refresh service, but the expiration time is not set
	ErrRefreshServiceLacksURL            = errors.New("credential request with refresh service lacks url")             // ErrRefreshServiceLacksURL means the credential request includes a refresh service, but the url is not set
	ErrSchemaNotFound                    = errors.New("schema not found")                                              // ErrSchemaNotFound Cannot retrieve the given schema from DB
	ErrUnsupportedDisplayMethodType      = errors.New("unsupported display method type")                               // ErrUnsupportedDisplayMethodType means the display method type is not supported
	ErrUnsupportedRefreshServiceType     = errors.New("unsupported refresh service type")                              // ErrUnsupportedRefreshServiceType means the refresh service type is not supported
	ErrWrongCredentialSubjectID          = errors.New("wrong format for credential subject ID")                        // ErrWrongCredentialSubjectID means the credential subject ID is wrong
	ErrAuthCredentialCannotBeRevoked     = errors.New("cannot delete the only remaining authentication credential. " +
		"An identity must have at least one credential") // ErrAuthCredentialCannotBeRevoked means the credential cannot be revoked
	ErrDisplayMethodNotFound = errors.New("display method not found") // ErrDisplayMethodNotFound Cannot retrieve the given display method
)

type claim struct {
	host string
	cfg  config.UniversalLinks

	icRepo                   ports.ClaimRepository
	identitySrv              ports.IdentityService
	mtService                ports.MtService
	qrService                ports.QrStoreService
	identityStateRepository  ports.IdentityStateRepository
	storage                  *db.Storage
	loader                   loader.DocumentLoader
	publisher                pubsub.Publisher
	ipfsClient               *shell.Shell
	revocationStatusResolver *revocationstatus.Resolver
	mediatypeManager         ports.MediaTypeManager
}

// NewClaim creates a new claim service
func NewClaim(repo ports.ClaimRepository, idenSrv ports.IdentityService, qrService ports.QrStoreService, mtService ports.MtService, identityStateRepository ports.IdentityStateRepository, ld loader.DocumentLoader, storage *db.Storage, host string, ps pubsub.Publisher, ipfsGatewayURL string, revocationStatusResolver *revocationstatus.Resolver, mediatypeManager ports.MediaTypeManager, cfg config.UniversalLinks) ports.ClaimService {
	s := &claim{
		host:                     host,
		icRepo:                   repo,
		identitySrv:              idenSrv,
		mtService:                mtService,
		qrService:                qrService,
		identityStateRepository:  identityStateRepository,
		storage:                  storage,
		loader:                   ld,
		publisher:                ps,
		revocationStatusResolver: revocationStatusResolver,
		mediatypeManager:         mediatypeManager,
		cfg:                      cfg,
	}
	if ipfsGatewayURL != "" {
		s.ipfsClient = shell.NewShell(ipfsGatewayURL)
	}
	return s
}

// Save creates a new claim
// 1.- Creates document
// 2.- Signature proof
// 3.- MerkelTree proof
func (c *claim) Save(ctx context.Context, req *ports.CreateClaimRequest) (*domain.Claim, error) {
	claim, err := c.CreateCredential(ctx, req)
	if err != nil {
		return nil, err
	}
	claim.ID, err = c.icRepo.Save(ctx, c.storage.Pgx, claim)
	if err != nil {
		return nil, err
	}
	if req.SignatureProof {
		err = c.publisher.Publish(ctx, event.CreateCredentialEvent, &event.CreateCredential{CredentialIDs: []string{claim.ID.String()}, IssuerID: req.DID.String()})
		if err != nil {
			log.Error(ctx, "publish CreateCredentialEvent", "err", err.Error(), "credential", claim.ID.String())
		}
	}

	return claim, nil
}

// GetRevoked returns all the revoked credentials for the given state
func (c *claim) GetRevoked(ctx context.Context, currentState string) ([]*domain.Claim, error) {
	return c.icRepo.GetRevoked(ctx, c.storage.Pgx, currentState)
}

// CreateCredential - Create a new Credential, but this method doesn't save it in the repository.
func (c *claim) CreateCredential(ctx context.Context, req *ports.CreateClaimRequest) (*domain.Claim, error) {
	if err := c.guardCreateClaimRequest(req); err != nil {
		log.Error(ctx, "create claim request validation", "req", req, "err", err)
		return nil, err
	}

	var nonce uint64
	var err error
	if req.RevNonce != nil {
		nonce = *req.RevNonce
	} else {
		nonce, err = common.Int64()
	}
	if err != nil {
		log.Error(ctx, "create a nonce", "err", err)
		return nil, err
	}

	schema, err := schemaPkg.LoadSchema(ctx, c.loader, req.Schema)
	if err != nil {
		log.Error(ctx, "loading schema", "err", err, "schema", req.Schema)
		return nil, ErrLoadingSchema
	}

	if schema.Metadata == nil {
		log.Error(ctx, "schema metadata is nil", "err", ErrProcessSchema)
		return nil, ErrProcessSchema
	}

	jsonLdContext, ok := schema.Metadata.Uris["jsonLdContext"].(string)
	if !ok {
		log.Error(ctx, "invalid jsonLdContext", "err", ErrJSONLdContext)
		return nil, ErrJSONLdContext
	}

	var vcID uuid.UUID
	if req.ClaimID != nil {
		vcID = *req.ClaimID
	} else {
		vcID, err = uuid.NewUUID()
		if err != nil {
			return nil, err
		}
	}

	vc, err := c.createVC(ctx, req, vcID, jsonLdContext, nonce)
	if err != nil {
		log.Error(ctx, "creating verifiable credential", "err", err)
		return nil, err
	}

	jsonLD, err := jsonschema.Load(ctx, jsonLdContext, c.loader)
	if err != nil {
		log.Error(ctx, "loading jsonLdContext", "err", err, "url", jsonLdContext)
		return nil, err
	}
	_, err = merklize.TypeIDFromContext(jsonLD.BytesNoErr(), req.Type)
	if err != nil {
		log.Error(ctx, "getting credential type", "err", err)
		return nil, err
	}
	opts := &processor.CoreClaimOptions{
		RevNonce:              nonce,
		MerklizedRootPosition: common.DefineMerklizedRootPosition(schema.Metadata, req.MerklizedRootPosition),
		Version:               req.Version,
		SubjectPosition:       req.SubjectPos,
		Updatable:             false,
	}
	if c.ipfsClient != nil {
		opts.MerklizerOpts = []merklize.MerklizeOption{merklize.WithDocumentLoader(c.loader)}
	}

	coreClaim, err := schemaPkg.Process(ctx, c.loader, req.Schema, vc, opts)
	if err != nil {
		log.Error(ctx, "credential subject attributes don't match the provided schema", "err", err)
		if errors.Is(err, schemaPkg.ErrParseClaim) {
			log.Error(ctx, "error parsing claim", "err", err)
			return nil, ErrParseClaim
		}
		if errors.Is(err, schemaPkg.ErrValidateData) {
			log.Error(ctx, "error validating data", "err", err)
			return nil, ErrInvalidCredentialSubject
		}
		if errors.Is(err, schemaPkg.ErrLoadSchema) {
			log.Error(ctx, "error loading schema", "err", err)
			return nil, ErrLoadingSchema
		}
		return nil, err
	}

	claim, err := domain.FromClaimer(coreClaim, req.Schema, req.Type)
	if err != nil {
		log.Error(ctx, "cannot obtain the claim from claimer", "err", err)
		return nil, err
	}

	issuerDIDString := req.DID.String()
	claim.Identifier = &issuerDIDString
	claim.Issuer = issuerDIDString
	claim.ID = vcID

	if req.SignatureProof {
		authClaim, err := c.GetAuthClaim(ctx, req.DID)
		if err != nil {
			log.Error(ctx, "cannot retrieve the auth claim", "err", err)
			return nil, err
		}

		proof, err := c.identitySrv.SignClaimEntry(ctx, authClaim, coreClaim)
		if err != nil {
			log.Error(ctx, "cannot sign claim entry", "err", err)
			return nil, err
		}

		authCs, err := authClaim.GetCredentialStatus()
		if err != nil {
			log.Error(ctx, "cannot get the auth claim credential status", "err", err)
			return nil, err
		}

		proof.IssuerData.CredentialStatus = authCs

		jsonSignatureProof, err := json.Marshal(proof)
		if err != nil {
			log.Error(ctx, "cannot encode the json signature proof", "err", err)
			return nil, err
		}
		err = claim.SignatureProof.Set(jsonSignatureProof)
		if err != nil {
			log.Error(ctx, "cannot set the json signature proof", "err", err)
			return nil, err
		}
	}

	err = claim.Data.Set(vc)
	if err != nil {
		log.Error(ctx, "cannot set the credential", "err", err)
		return nil, err
	}

	err = claim.CredentialStatus.Set(vc.CredentialStatus)
	if err != nil {
		log.Error(ctx, "cannot set the credential status", "err", err)
		return nil, err
	}

	claim.MtProof = req.MTProof
	claim.LinkID = req.LinkID
	claim.CreatedAt = *vc.IssuanceDate
	return claim, nil
}

func (c *claim) Revoke(ctx context.Context, id w3c.DID, nonce uint64, description string) error {
	return c.revoke(ctx, &id, nonce, description, c.storage.Pgx)
}

func (c *claim) RevokeAllFromConnection(ctx context.Context, connID uuid.UUID, issuerID w3c.DID) error {
	credentials, err := c.icRepo.GetNonRevokedByConnectionAndIssuerID(ctx, c.storage.Pgx, connID, issuerID)
	if err != nil {
		return err
	}

	return c.storage.Pgx.BeginFunc(ctx,
		func(tx pgx.Tx) error {
			for _, credential := range credentials {
				err := c.revoke(ctx, &issuerID, uint64(credential.RevNonce), "", tx)
				if err != nil {
					return err
				}
			}
			return nil
		})
}

func (c *claim) Delete(ctx context.Context, issuerDID *w3c.DID, id uuid.UUID) error {
	claim, err := c.icRepo.GetByIdAndIssuer(ctx, c.storage.Pgx, issuerDID, id)
	if err != nil {
		if errors.Is(err, repositories.ErrClaimDoesNotExist) {
			return ErrCredentialNotFound
		}
		return err
	}

	claims := make([]*domain.Claim, 1)
	claims[0] = claim

	authHash, err := core.AuthSchemaHash.MarshalText()
	if err != nil {
		return err
	}

	// check if the nonce can be deleted
	canBeRevoked, err := c.canRevokeNonce(ctx, issuerDID, c.storage.Pgx, claims, uint64(claim.RevNonce), string(authHash))
	if err != nil {
		return fmt.Errorf("error checking if the nonce can be revoked: %w", err)
	}
	if !canBeRevoked {
		return ErrAuthCredentialCannotBeRevoked
	}

	err = c.icRepo.Delete(ctx, c.storage.Pgx, id)
	if err != nil {
		if errors.Is(err, repositories.ErrClaimDoesNotExist) {
			return ErrCredentialNotFound
		}
		return err
	}

	return nil
}

func (c *claim) GetByID(ctx context.Context, issID *w3c.DID, id uuid.UUID) (*domain.Claim, error) {
	claim, err := c.icRepo.GetByIdAndIssuer(ctx, c.storage.Pgx, issID, id)
	if err != nil {
		if errors.Is(err, repositories.ErrClaimDoesNotExist) {
			return nil, ErrCredentialNotFound
		}
		return nil, err
	}

	return claim, nil
}

// GetCredentialQrCode creates a credential QR code for the given credential and returns the QR Link to be used
func (c *claim) GetCredentialQrCode(ctx context.Context, issID *w3c.DID, id uuid.UUID, hostURL string) (*ports.GetCredentialQrCodeResponse, error) {
	getCredentialType := func(claim domain.Claim) string {
		credentialType := claim.SchemaType
		const schemaParts = 2
		parse := strings.Split(credentialType, "#")
		if len(parse) != schemaParts {
			return credentialType
		}
		return parse[1]
	}

	claim, err := c.GetByID(ctx, issID, id)
	if err != nil {
		log.Error(ctx, "getCredentialQrQrCode: get credential by id", "err", err, "id", id)
		return nil, err
	}

	if !claim.ValidProof() {
		log.Error(ctx, "getCredentialQrQrCode: invalid proof", "id", id)
		return nil, ErrEmptyMTPProof
	}
	credID := uuid.New()
	qrCode := protocol.CredentialsOfferMessage{
		Body: protocol.CredentialsOfferMessageBody{
			Credentials: []protocol.CredentialOffer{
				{
					Description: getCredentialType(*claim),
					ID:          claim.ID.String(),
				},
			},
			URL: fmt.Sprintf(ports.AgentUrl, strings.TrimSuffix(hostURL, "/")),
		},
		From:     claim.Issuer,
		ID:       credID.String(),
		ThreadID: credID.String(),
		To:       claim.OtherIdentifier,
		Typ:      packers.MediaTypePlainMessage,
		Type:     protocol.CredentialOfferMessageType,
	}

	raw, err := json.Marshal(qrCode)
	if err != nil {
		log.Error(ctx, "getCredentialQrQrCode: marshal qr code", "err", err)
		return nil, err
	}
	qrID, err := c.qrService.Store(ctx, raw, DefaultQRBodyTTL)
	if err != nil {
		log.Error(ctx, "getCredentialQrQrCode: store qr code", "err", err)
		return nil, err
	}
	return &ports.GetCredentialQrCodeResponse{
		DeepLink:      qrlink.NewDeepLink(hostURL, qrID, nil),
		UniversalLink: qrlink.NewUniversal(c.cfg.BaseUrl, hostURL, qrID, nil),
		QrRaw:         string(raw),
		SchemaType:    getCredentialType(*claim),
		QrID:          qrID,
	}, nil
}

func (c *claim) Agent(ctx context.Context, req *ports.AgentRequest, mediatype iden3comm.MediaType) (*iden3comm.BasicMessage, error) {
	if req.UserDID == nil {
		return nil, fmt.Errorf("'from' field cannot be empty")
	}

	if req.IssuerDID == nil {
		return nil, fmt.Errorf("'to' field cannot be empty")
	}

	if !c.mediatypeManager.AllowMediaType(req.Type, mediatype) {
		err := fmt.Errorf("unsupported media type '%s' for message type '%s'", mediatype, req.Type)
		log.Error(ctx, "agent: unsupported media type", "err", err)
		return nil, err
	}

	exists, err := c.identitySrv.Exists(ctx, *req.IssuerDID)
	if err != nil {
		log.Error(ctx, "loading issuer identity", "err", err, "issuerDID", req.IssuerDID)
		return nil, err
	}

	if !exists {
		log.Warn(ctx, "issuer not found", "issuerDID", req.IssuerDID)
		return nil, fmt.Errorf("cannot proceed with this identity, not found")
	}

	switch req.Type {
	case protocol.CredentialFetchRequestMessageType:
		return c.getAgentCredential(ctx, req)
	case protocol.RevocationStatusRequestMessageType:
		return c.getRevocationStatus(ctx, req)
	default:
		return nil, errors.New("invalid type")
	}
}

func (c *claim) GetAuthClaim(ctx context.Context, did *w3c.DID) (*domain.Claim, error) {
	authHash, err := core.AuthSchemaHash.MarshalText()
	if err != nil {
		return nil, err
	}
	return c.icRepo.FindOneClaimBySchemaHash(ctx, c.storage.Pgx, did, string(authHash))
}

// GetFirstNonRevokedAuthClaim returns the first non-revoked authentication claim for the given DID. The AuthClaim may not be published
func (c *claim) GetFirstNonRevokedAuthClaim(ctx context.Context, did *w3c.DID) (*domain.Claim, error) {
	authHash, err := core.AuthSchemaHash.MarshalText()
	if err != nil {
		return nil, err
	}
	authClaims, err := c.icRepo.GetAuthCoreClaims(ctx, c.storage.Pgx, did, string(authHash))
	if err != nil {
		return nil, err
	}

	return authClaims[0], nil
}

func (c *claim) GetAll(ctx context.Context, did w3c.DID, filter *ports.ClaimsFilter) ([]*domain.Claim, uint, error) {
	claims, total, err := c.icRepo.GetAllByIssuerID(ctx, c.storage.Pgx, did, filter)
	if err != nil {
		if errors.Is(err, repositories.ErrClaimDoesNotExist) {
			return nil, 0, ErrCredentialNotFound
		}
		return nil, 0, err
	}

	return claims, total, nil
}

func (c *claim) GetRevocationStatus(ctx context.Context, issuerDID w3c.DID, nonce uint64) (*verifiable.RevocationStatus, error) {
	rID := new(big.Int).SetUint64(nonce)
	revocationStatus := &verifiable.RevocationStatus{}

	state, err := c.identityStateRepository.GetLatestStateByIdentifier(ctx, c.storage.Pgx, &issuerDID)
	if err != nil {
		return nil, err
	}

	revocationStatus.Issuer.State = state.State
	revocationStatus.Issuer.ClaimsTreeRoot = state.ClaimsTreeRoot
	revocationStatus.Issuer.RevocationTreeRoot = state.RevocationTreeRoot
	revocationStatus.Issuer.RootOfRoots = state.RootOfRoots

	if state.RevocationTreeRoot == nil {
		var mtp *merkletree.Proof
		mtp, err = merkletree.NewProofFromData(false, nil, nil)
		if err != nil {
			return nil, err
		}
		revocationStatus.MTP = *mtp
		return revocationStatus, nil
	}

	revocationTreeHash, err := merkletree.NewHashFromHex(*state.RevocationTreeRoot)
	if err != nil {
		return nil, err
	}
	identityTrees, err := c.mtService.GetIdentityMerkleTrees(ctx, c.storage.Pgx, &issuerDID)
	if err != nil {
		return nil, err
	}

	// revocation / non revocation MTP for the latest identity state
	proof, err := identityTrees.GenerateRevocationProof(ctx, rID, revocationTreeHash)
	if err != nil {
		return nil, err
	}

	revocationStatus.MTP = *proof

	return revocationStatus, nil
}

func (c *claim) GetAuthClaimForPublishing(ctx context.Context, did *w3c.DID, state string) (*domain.Claim, error) {
	authHash, err := core.AuthSchemaHash.MarshalText()
	if err != nil {
		return nil, err
	}

	validAuthClaims, err := c.icRepo.GetAuthClaimsForPublishing(ctx, c.storage.Pgx, did, state, string(authHash))
	if err != nil {
		return nil, err
	}
	if len(validAuthClaims) == 0 {
		return nil, errors.New("no auth claims for publishing")
	}

	return validAuthClaims[0], nil
}

// UpdateClaimsMTPAndState update identity status and claim MTP
func (c *claim) UpdateClaimsMTPAndState(ctx context.Context, currentState *domain.IdentityState) error {
	did, err := w3c.ParseDID(currentState.Identifier)
	if err != nil {
		return err
	}

	iTrees, err := c.mtService.GetIdentityMerkleTrees(ctx, c.storage.Pgx, did)
	if err != nil {
		return err
	}

	claimsTree, err := iTrees.ClaimsTree()
	if err != nil {
		return err
	}

	currState, err := merkletree.NewHashFromHex(*currentState.State)
	if err != nil {
		return err
	}

	claims, err := c.icRepo.GetAllByStateWithMTProof(ctx, c.storage.Pgx, did, currState)
	if err != nil {
		return err
	}

	for i := range claims {
		var index *big.Int
		var coreClaimHex string
		coreClaim := claims[i].CoreClaim.Get()
		index, err = coreClaim.HIndex()
		if err != nil {
			return err
		}
		var proof *merkletree.Proof
		proof, _, err = claimsTree.GenerateProof(ctx, index, claimsTree.Root())
		if err != nil {
			return err
		}
		coreClaimHex, err = coreClaim.Hex()
		if err != nil {
			return err
		}
		mtpProof := verifiable.Iden3SparseMerkleTreeProof{
			Type: verifiable.Iden3SparseMerkleTreeProofType,
			IssuerData: verifiable.IssuerData{
				ID: did.String(),
				State: verifiable.State{
					RootOfRoots:        currentState.RootOfRoots,
					ClaimsTreeRoot:     currentState.ClaimsTreeRoot,
					RevocationTreeRoot: currentState.RevocationTreeRoot,
					Value:              currentState.State,
					BlockTimestamp:     currentState.BlockTimestamp,
					TxID:               currentState.TxID,
					BlockNumber:        currentState.BlockNumber,
				},
			},
			CoreClaim: coreClaimHex,
			MTP:       proof,
		}

		var jsonProof []byte
		jsonProof, err = json.Marshal(mtpProof)
		if err != nil {
			return fmt.Errorf("can't marshal proof: %w", err)
		}

		var affected int64
		err = claims[i].MTPProof.Set(jsonProof)
		if err != nil {
			return fmt.Errorf("failed set mtp proof: %w", err)
		}
		affected, err = c.icRepo.UpdateClaimMTP(ctx, c.storage.Pgx, &claims[i])
		if err != nil {
			return fmt.Errorf("can't update claim mtp:  %w", err)
		}
		if affected == 0 {
			return fmt.Errorf("claim has not been updated %v", claims[i])
		}
	}
	_, err = c.identityStateRepository.UpdateState(ctx, c.storage.Pgx, currentState)
	if err != nil {
		return fmt.Errorf("can't update identity state: %w", err)
	}

	return nil
}

func (c *claim) GetByStateIDWithMTPProof(ctx context.Context, did *w3c.DID, state string) ([]*domain.Claim, error) {
	return c.icRepo.GetByStateIDWithMTPProof(ctx, c.storage.Pgx, did, state)
}

// GetAuthCredentials returns the auth credentials for the given identifier
// The auth credentials are the credentials that are used to sign other credentials
// The credentials can have mtp proof or not.
// The credentials are not revoked
func (c *claim) GetAuthCredentials(ctx context.Context, identifier *w3c.DID) ([]*domain.Claim, error) {
	authHash, err := core.AuthSchemaHash.MarshalText()
	if err != nil {
		return nil, err
	}
	return c.icRepo.GetAuthCoreClaims(ctx, c.storage.Pgx, identifier, string(authHash))
}

// GetAuthCredentialByPublicKey returns the auth credential with the given public key
func (c *claim) GetAuthCredentialByPublicKey(ctx context.Context, identifier *w3c.DID, publicKey []byte) (*domain.Claim, error) {
	authCredentials, err := c.GetAuthCredentials(ctx, identifier)
	if err != nil {
		log.Error(ctx, "failed to get auth credentials", "err", err)
		return nil, err
	}
	for _, authCredential := range authCredentials {
		if utils.GetPublicKeyFromClaim(authCredential).Equal(publicKey) {
			return authCredential, nil
		}
	}
	return nil, nil
}

func (c *claim) revoke(ctx context.Context, did *w3c.DID, nonce uint64, description string, querier db.Querier) error {
	authHash, err := core.AuthSchemaHash.MarshalText()
	if err != nil {
		return err
	}

	// get the claims to revoke by nonce
	claimsToRevoke, err := c.icRepo.GetByRevocationNonce(ctx, querier, did, domain.RevNonceUint64(nonce))
	if err != nil {
		log.Error(ctx, "error getting the claim by revocation nonce", "err", err)
	}

	// check if the nonce can be revoked
	canBeRevoked, err := c.canRevokeNonce(ctx, did, querier, claimsToRevoke, nonce, string(authHash))
	if err != nil {
		return fmt.Errorf("error checking if the nonce can be revoked: %w", err)
	}
	if !canBeRevoked {
		return ErrAuthCredentialCannotBeRevoked
	}

	rID := new(big.Int).SetUint64(nonce)
	revocation := domain.Revocation{
		Identifier:  did.String(),
		Nonce:       domain.RevNonceUint64(nonce),
		Version:     0,
		Status:      0,
		Description: description,
	}

	identityTrees, err := c.mtService.GetIdentityMerkleTrees(ctx, querier, did)
	if err != nil {
		return fmt.Errorf("error getting merkle trees: %w", err)
	}

	err = identityTrees.RevokeClaim(ctx, rID)
	if err != nil {
		return fmt.Errorf("error revoking the claim: %w", err)
	}

	var claims []*domain.Claim
	claims, err = c.icRepo.GetByRevocationNonce(ctx, querier, did, domain.RevNonceUint64(nonce))
	if err != nil {
		if errors.Is(err, repositories.ErrClaimDoesNotExist) {
			return err
		}
		return fmt.Errorf("error getting the claim by revocation nonce: %w", err)
	}

	err = c.storage.Pgx.BeginFunc(ctx,
		func(tx pgx.Tx) error {
			for _, claim := range claims {
				claim.Revoked = true
				_, err = c.icRepo.Save(ctx, tx, claim)
				if err != nil {
					log.Error(ctx, "error saving the claim", "err", err)
					return fmt.Errorf("error saving the claim: %w", err)
				}
			}

			return c.icRepo.RevokeNonce(ctx, tx, &revocation)
		})
	if err != nil {
		log.Error(ctx, "error saving the revoked claims", "err", err)
		return err
	}

	return nil
}

// canRevokeNonce checks if the nonce can be revoked
func (c *claim) canRevokeNonce(ctx context.Context, did *w3c.DID, querier db.Querier, claimsToRevoke []*domain.Claim, nonce uint64, authHash string) (bool, error) {
	checkAuthCredentials := false
	for _, claim := range claimsToRevoke {
		if claim.EqualToSchemaHash(authHash) {
			checkAuthCredentials = true
			break
		}
	}

	canBeRevoked := true
	if checkAuthCredentials {
		authCredentials, err := c.icRepo.FindClaimsBySchemaHash(ctx, querier, did, authHash)
		if err != nil {
			return false, fmt.Errorf("error getting the auth credentials: %w", err)
		}
		if len(authCredentials) == 1 {
			return false, nil
		}
		canBeRevoked = false
		for _, authCredential := range authCredentials {
			if authCredential.RevNonce != domain.RevNonceUint64(nonce) {
				canBeRevoked = true
			}
		}
	}
	return canBeRevoked, nil
}

func (c *claim) getRevocationStatus(ctx context.Context, basicMessage *ports.AgentRequest) (*iden3comm.BasicMessage, error) {
	revData := &protocol.RevocationStatusRequestMessageBody{}
	err := json.Unmarshal(basicMessage.Body, revData)
	if err != nil {
		return nil, fmt.Errorf("invalid revocation request body: %w", err)
	}

	var revStatus *verifiable.RevocationStatus
	revStatus, err = c.GetRevocationStatus(ctx, *basicMessage.IssuerDID, revData.RevocationNonce)
	if err != nil {
		return nil, fmt.Errorf("failed get revocation status: %w", err)
	}

	body, err := json.Marshal(protocol.RevocationStatusResponseMessageBody{RevocationStatus: *revStatus})
	if err != nil {
		log.Error(ctx, "marshaling body", "err", err)
		return nil, err
	}

	return &iden3comm.BasicMessage{
		ID:       uuid.NewString(),
		Type:     protocol.RevocationStatusResponseMessageType,
		ThreadID: basicMessage.ThreadID,
		Body:     body,
		From:     basicMessage.IssuerDID.String(),
		To:       basicMessage.UserDID.String(),
		Typ:      packers.MediaTypePlainMessage,
	}, nil
}

func (c *claim) getAgentCredential(ctx context.Context, basicMessage *ports.AgentRequest) (*iden3comm.BasicMessage, error) {
	fetchRequestBody := &protocol.CredentialFetchRequestMessageBody{}
	err := json.Unmarshal(basicMessage.Body, fetchRequestBody)
	if err != nil {
		log.Error(ctx, "unmarshalling agent body", "err", err)
		return nil, fmt.Errorf("invalid credential fetch request body: %w", err)
	}

	claimID, err := urn.UUIDFromURNString(fetchRequestBody.ID)
	if err != nil {
		claimID, err = uuid.Parse(fetchRequestBody.ID)
		if err != nil {
			log.Error(ctx, "wrong claimID in agent request body", "err", err)
			return nil, fmt.Errorf("invalid claim ID")
		}
	}

	claim, err := c.icRepo.GetByIdAndIssuer(ctx, c.storage.Pgx, basicMessage.IssuerDID, claimID)
	if err != nil {
		log.Error(ctx, "loading claim", "err", err)
		return nil, fmt.Errorf("failed get claim by claimID: %w", err)
	}

	if claim.OtherIdentifier != basicMessage.UserDID.String() {
		err := fmt.Errorf("claim doesn't relate to sender")
		log.Error(ctx, "claim doesn't relate to sender", err, "claimID", claim.ID)
		return nil, err
	}

	vc, err := schemaPkg.FromClaimModelToW3CCredential(*claim)
	if err != nil {
		log.Error(ctx, "creating W3 credential", "err", err)
		return nil, fmt.Errorf("failed to convert claim to  w3cCredential: %w", err)
	}

	body, err := json.Marshal(protocol.IssuanceMessageBody{Credential: *vc})
	if err != nil {
		log.Error(ctx, "marshaling body", "err", err)
		return nil, err
	}
	return &iden3comm.BasicMessage{
		ID:       uuid.NewString(),
		Typ:      packers.MediaTypePlainMessage,
		Type:     protocol.CredentialIssuanceResponseMessageType,
		ThreadID: basicMessage.ThreadID,
		Body:     body,
		From:     basicMessage.IssuerDID.String(),
		To:       basicMessage.UserDID.String(),
	}, err
}

func (c *claim) createVC(ctx context.Context, claimReq *ports.CreateClaimRequest, vcID uuid.UUID, jsonLdContext string, nonce uint64) (verifiable.W3CCredential, error) {
	vCredential, err := c.newVerifiableCredential(ctx, claimReq, vcID, jsonLdContext, nonce) // create vc credential
	if err != nil {
		return verifiable.W3CCredential{}, err
	}

	return vCredential, nil
}

func (c *claim) guardCreateClaimRequest(req *ports.CreateClaimRequest) error {
	type guardFunc func() error

	guards := []guardFunc{
		// check if schema's URL is valid
		func() error {
			if _, err := url.ParseRequestURI(req.Schema); err != nil {
				return ErrMalformedURL
			}
			return nil
		},
		// check if refresh service has supported type
		func() error {
			if req.RefreshService == nil {
				return nil
			}
			if req.Expiration == nil {
				return ErrRefreshServiceLacksExpirationTime
			}
			if req.RefreshService.ID == "" {
				return ErrRefreshServiceLacksURL
			}
			_, err := url.ParseRequestURI(req.RefreshService.ID)
			if err != nil {
				return ErrRefreshServiceLacksURL
			}

			switch req.RefreshService.Type {
			case verifiable.Iden3RefreshService2023:
				return nil
			default:
				return ErrUnsupportedRefreshServiceType
			}
		},
		// check display method in correct uri
		func() error {
			if req.DisplayMethod == nil {
				return nil
			}
			if req.DisplayMethod.ID == "" {
				return ErrDisplayMethodLacksURL
			}
			_, err := url.ParseRequestURI(req.DisplayMethod.ID)
			if err != nil {
				return ErrDisplayMethodLacksURL
			}

			switch req.DisplayMethod.Type {
			case verifiable.Iden3BasicDisplayMethodV1:
				return nil
			default:
				return ErrUnsupportedDisplayMethodType
			}
		},
		// check identity
		func() error {
			if _, found := req.CredentialSubject["id"]; found {
				did, ok := req.CredentialSubject["id"].(string)
				if !ok {
					return ErrWrongCredentialSubjectID
				}
				_, err := w3c.ParseDID(did)
				if err != nil {
					return ErrWrongCredentialSubjectID
				}
			}
			return nil
		},
	}
	if req.RefreshService != nil {
		if req.Expiration == nil {
			return ErrRefreshServiceLacksExpirationTime
		}
		if req.RefreshService.Type != verifiable.Iden3RefreshService2023 {
			return ErrUnsupportedRefreshServiceType
		}
	}

	for _, guard := range guards {
		if err := guard(); err != nil {
			return err
		}
	}

	return nil
}

func (c *claim) newVerifiableCredential(ctx context.Context, claimReq *ports.CreateClaimRequest, vcID uuid.UUID, jsonLdContext string, nonce uint64) (verifiable.W3CCredential, error) {
	credentialCtx := []string{verifiable.JSONLDSchemaW3CCredential2018, verifiable.JSONLDSchemaIden3Credential, jsonLdContext}
	credentialType := []string{verifiable.TypeW3CVerifiableCredential, claimReq.Type}

	credentialSubject := claimReq.CredentialSubject

	if idSubject, ok := credentialSubject["id"].(string); ok {
		did, err := w3c.ParseDID(idSubject)
		if err != nil {
			return verifiable.W3CCredential{}, err
		}
		credentialSubject["id"] = did.String()
	}

	credentialSubject["type"] = claimReq.Type

	latestIssuerState, err := c.identitySrv.GetLatestStateByID(ctx, *claimReq.DID)
	if err != nil {
		log.Error(ctx, "getting latest issuer state", "err", err)
		return verifiable.W3CCredential{}, err
	}
	cs, err := c.revocationStatusResolver.GetCredentialRevocationStatus(ctx, *claimReq.DID, nonce, *latestIssuerState.State, claimReq.CredentialStatusType)
	if err != nil {
		log.Error(ctx, "getting credential status", "err", err)
		return verifiable.W3CCredential{}, err
	}

	if claimReq.DisplayMethod != nil {
		credentialCtx = append(credentialCtx, verifiable.JSONLDSchemaIden3DisplayMethod)
	}

	issuanceDate := time.Now().UTC()
	return verifiable.W3CCredential{
		ID:                string(c.buildCredentialID(vcID)),
		Context:           credentialCtx,
		Type:              credentialType,
		Expiration:        claimReq.Expiration,
		IssuanceDate:      &issuanceDate,
		CredentialSubject: credentialSubject,
		Issuer:            claimReq.DID.String(),
		CredentialSchema: verifiable.CredentialSchema{
			ID:   claimReq.Schema,
			Type: verifiable.JSONSchema2023,
		},
		CredentialStatus: cs,
		RefreshService:   claimReq.RefreshService,
		DisplayMethod:    claimReq.DisplayMethod,
	}, nil
}

func (c *claim) buildCredentialID(credID uuid.UUID) urn.URN {
	return urn.FromUUID(credID)
}
