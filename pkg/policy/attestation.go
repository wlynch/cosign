//
// Copyright 2022 The Sigstore Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package policy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/in-toto/in-toto-golang/in_toto"
	"github.com/pkg/errors"
	"github.com/sigstore/cosign/pkg/oci"

	"github.com/sigstore/cosign/cmd/cosign/cli/options"
	"github.com/sigstore/cosign/pkg/cosign/attestation"
)

// AttestationToPayloadJSON takes in a verified Attestation (oci.Signature) and
// marshals it into a JSON depending on the payload that's then consumable
// by policy engine like cue, rego, etc.
//
// Anything fed here must have been validated with either
// `VerifyLocalImageAttestations` or `VerifyImageAttestations`
//
// If there's no error, and payload is empty means the predicateType did not
// match the attestation.
func AttestationToPayloadJSON(ctx context.Context, predicateType string, verifiedAttestation oci.Signature) ([]byte, error) {
	// Check the predicate up front, no point in wasting time if it's invalid.
	predicateURI, ok := options.PredicateTypeMap[predicateType]
	if !ok {
		return nil, fmt.Errorf("invalid predicate type: %s", predicateType)
	}

	var payloadData map[string]interface{}

	p, err := verifiedAttestation.Payload()
	if err != nil {
		return nil, errors.Wrap(err, "getting payload")
	}

	err = json.Unmarshal(p, &payloadData)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshaling payload data")
	}

	var decodedPayload []byte
	if val, ok := payloadData["payload"]; ok {
		decodedPayload, err = base64.StdEncoding.DecodeString(val.(string))
		if err != nil {
			return nil, errors.Wrap(err, "decoding payload")
		}
	} else {
		return nil, fmt.Errorf("could not find payload in payload data")
	}

	// Only apply the policy against the requested predicate type
	var statement in_toto.Statement
	if err := json.Unmarshal(decodedPayload, &statement); err != nil {
		return nil, fmt.Errorf("unmarshal in-toto statement: %w", err)
	}
	if statement.PredicateType != predicateURI {
		// This is not the predicate we're looking for, so skip it.
		return nil, nil
	}

	// NB: In many (all?) of these cases, we could just return the
	// 'json.Marshal', but we check for errors here to decorate them
	// with more meaningful error message.
	var payload []byte
	switch predicateType {
	case options.PredicateCustom:
		payload, err = json.Marshal(statement)
		if err != nil {
			return nil, errors.Wrap(err, "generating CosignStatement")
		}
	case options.PredicateLink:
		var linkStatement in_toto.LinkStatement
		if err := json.Unmarshal(decodedPayload, &linkStatement); err != nil {
			return nil, errors.Wrap(err, "unmarshaling LinkStatement")
		}
		payload, err = json.Marshal(linkStatement)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling LinkStatement")
		}
	case options.PredicateSLSA:
		var slsaProvenanceStatement in_toto.ProvenanceStatement
		if err := json.Unmarshal(decodedPayload, &slsaProvenanceStatement); err != nil {
			return nil, errors.Wrap(err, "unmarshaling ProvenanceStatement")
		}
		payload, err = json.Marshal(slsaProvenanceStatement)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling ProvenanceStatement")
		}
	case options.PredicateSPDX:
		var spdxStatement in_toto.SPDXStatement
		if err := json.Unmarshal(decodedPayload, &spdxStatement); err != nil {
			return nil, errors.Wrap(err, "unmarshaling SPDXStatement")
		}
		payload, err = json.Marshal(spdxStatement)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling SPDXStatement")
		}
	case options.PredicateVuln:
		var vulnStatement attestation.CosignVulnStatement
		if err := json.Unmarshal(decodedPayload, &vulnStatement); err != nil {
			return nil, errors.Wrap(err, "unmarshaling CosignVulnStatement")
		}
		payload, err = json.Marshal(vulnStatement)
		if err != nil {
			return nil, errors.Wrap(err, "marshaling CosignVulnStatement")
		}
	default:
		return nil, fmt.Errorf("unsupported predicate type: %s", predicateType)
	}
	return payload, nil
}
