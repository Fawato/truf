package dwolla

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/trufflesecurity/trufflehog/v3/pkg/common"
	"github.com/trufflesecurity/trufflehog/v3/pkg/detectors"
	"github.com/trufflesecurity/trufflehog/v3/pkg/pb/detectorspb"
)

type Scanner struct{}

// Ensure the Scanner satisfies the interface at compile time.
var _ detectors.Detector = (*Scanner)(nil)

var (
	client = common.SaneHttpClient()

	// Make sure that your group is surrounded in boundary characters such as below to reduce false positives.
	idPat     = regexp.MustCompile(detectors.PrefixRegex([]string{"dwolla"}) + `\b([a-zA-Z-0-9]{50})\b`)
	secretPat = regexp.MustCompile(detectors.PrefixRegex([]string{"dwolla"}) + `\b([a-zA-Z-0-9]{50})\b`)
)

// Keywords are used for efficiently pre-filtering chunks.
// Use identifiers in the secret preferably, or the provider name.
func (s Scanner) Keywords() []string {
	return []string{"dwolla"}
}

// FromData will find and optionally verify Dwolla secrets in a given set of bytes.
func (s Scanner) FromData(ctx context.Context, verify bool, data []byte) (results []detectors.Result, err error) {
	dataStr := string(data)

	idMatches := idPat.FindAllStringSubmatch(dataStr, -1)
	secretMatches := secretPat.FindAllStringSubmatch(dataStr, -1)

	for _, match := range idMatches {
		if len(match) != 2 {
			continue
		}

		idMatch := strings.TrimSpace(match[1])

		for _, secret := range secretMatches {
			if len(secret) != 2 {
				continue
			}

			secretMatch := strings.TrimSpace(secret[1])

			s1 := detectors.Result{
				DetectorType: detectorspb.DetectorType_Dwolla,
				Raw:          []byte(idMatch),
				RawV2:        []byte(idMatch + secretMatch),
			}

			if verify {
				data := fmt.Sprintf("%s:%s", idMatch, secretMatch)
				encoded := b64.StdEncoding.EncodeToString([]byte(data))
				payload := strings.NewReader("grant_type=client_credentials")

				req, err := http.NewRequestWithContext(ctx, "POST", "https://api-sandbox.dwolla.com/token", payload)
				if err != nil {
					continue
				}
				req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Add("Authorization", fmt.Sprintf("Basic %s", encoded))

				res, err := client.Do(req)
				if err == nil {
					defer res.Body.Close()
					if res.StatusCode >= 200 && res.StatusCode < 300 {
						s1.Verified = true
					} else {
						// This function will check false positives for common test words, but also it will make sure the key appears 'random' enough to be a real key.
						if detectors.IsKnownFalsePositive(idMatch, detectors.DefaultFalsePositives, true) {
							continue
						}
					}
				}
			}

			results = append(results, s1)
		}
	}

	return results, nil
}
