package adfs

import (
	"encoding/base64"
	"encoding/json"
	"github.com/vouch/vouch-proxy/handlers/common"
	"github.com/vouch/vouch-proxy/pkg/cfg"
	"github.com/vouch/vouch-proxy/pkg/structs"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type adfsTokenRes struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IDToken     string `json:"id_token"`
	ExpiresIn   int64  `json:"expires_in"` // relative seconds from now
}

var (
	log = cfg.Cfg.Logger
)

// More info: https://docs.microsoft.com/en-us/windows-server/identity/ad-fs/overview/ad-fs-scenarios-for-developers#supported-scenarios
func GetUserInfoFromADFS(r *http.Request, user *structs.User, customClaims *structs.CustomClaims, ptokens *structs.PTokens) (rerr error) {
	code := r.URL.Query().Get("code")
	log.Debugf("code: %s", code)

	formData := url.Values{}
	formData.Set("code", code)
	formData.Set("grant_type", "authorization_code")
	formData.Set("resource", cfg.GenOAuth.RedirectURL)
	formData.Set("client_id", cfg.GenOAuth.ClientID)
	formData.Set("redirect_uri", cfg.GenOAuth.RedirectURL)
	if cfg.GenOAuth.ClientSecret != "" {
		formData.Set("client_secret", cfg.GenOAuth.ClientSecret)
	}
	req, err := http.NewRequest("POST", cfg.GenOAuth.TokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(formData.Encode())))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	userinfo, err := client.Do(req)

	if err != nil {
		return err
	}
	defer func() {
		if err := userinfo.Body.Close(); err != nil {
			rerr = err
		}
	}()

	data, _ := ioutil.ReadAll(userinfo.Body)
	tokenRes := adfsTokenRes{}

	if err := json.Unmarshal(data, &tokenRes); err != nil {
		log.Errorf("oauth2: cannot fetch token: %v", err)
		return nil
	}

	ptokens.PAccessToken = string(tokenRes.AccessToken)
	ptokens.PIdToken = string(tokenRes.IDToken)

	s := strings.Split(tokenRes.IDToken, ".")
	if len(s) < 2 {
		log.Error("jws: invalid token received")
		return nil
	}

	idToken, err := base64.RawURLEncoding.DecodeString(s[1])
	if err != nil {
		log.Error(err)
		return nil
	}
	log.Debugf("idToken: %+v", string(idToken))

	adfsUser := structs.ADFSUser{}
	json.Unmarshal([]byte(idToken), &adfsUser)
	log.Infof("adfs adfsUser: %+v", adfsUser)
	// data contains an access token, refresh token, and id token
	// Please note that in order for custom claims to work you MUST set allatclaims in ADFS to be passed
	// https://oktotechnologies.ca/2018/08/26/adfs-openidconnect-configuration/
	if err = common.MapClaims([]byte(idToken), customClaims); err != nil {
		log.Error(err)
		return err
	}
	adfsUser.PrepareUserData()
	var rxEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")

	if len(adfsUser.Email) == 0 {
		// If the email is blank, we will try to determine if the UPN is an email.
		if rxEmail.MatchString(adfsUser.UPN) {
			// Set the email from UPN if there is a valid email present.
			adfsUser.Email = adfsUser.UPN
		}
	}
	user.Username = adfsUser.Username
	user.Email = adfsUser.Email
	log.Debugf("User Obj: %+v", user)
	return nil
}