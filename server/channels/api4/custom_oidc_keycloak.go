package api4

import (
	"context"
	"log"
	"net/http"
    "regexp"
	"github.com/mattermost/mattermost/server/v8/channels/utils"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mattermost/mattermost/server/public/model"
     "strings"
	"os"
	"golang.org/x/oauth2"
)

var (
	keycloakVerifier     *oidc.IDTokenVerifier
	keycloakOAuth2Config oauth2.Config
	keycloakStateStore = make(map[string]string) // state → desktop_token
)


func (api *API) keycloakLoginStart(c *Context, w http.ResponseWriter, r *http.Request) {
	desktopToken := r.URL.Query().Get("desktop_token")

	state := model.NewId() 
	if desktopToken != "" {
		keycloakStateStore[state] = desktopToken
	}

	url := keycloakOAuth2Config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

func (api *API) keycloakLoginComplete(c *Context, w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	token, err := keycloakOAuth2Config.Exchange(ctx, code)
	if err != nil {
		http.Error(w, "Token exchange error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Missing id_token", http.StatusBadRequest)
		return
	}

	idToken, err := keycloakVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, "ID token verify failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	var claims struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		GivenName string `json:"given_name"`
		FamilyName string `json:"family_name"`
		PreferredUsername string `json:"preferred_username"`
		Position  string `json:"position"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Invalid claims", http.StatusInternalServerError)
		return
	}

	user, appErr := c.App.GetUserByEmail(claims.Email)
	if appErr != nil {
		fullName := claims.Name
		if fullName == "" && claims.GivenName != "" && claims.FamilyName != "" {
			fullName = claims.GivenName + " " + claims.FamilyName
		}

		username := claims.PreferredUsername
		if username == "" {
			username = generateUsername(claims.Email)
		}

		user = &model.User{
			Email:     claims.Email,
			Username:  username,
			Password:  model.NewId(),
			FirstName:  fullName,
			Position:  claims.Position,
		}

		user, appErr = c.App.CreateUser(c.AppContext, user)
		if appErr != nil {
			http.Error(w, "User create error: "+appErr.Error(), http.StatusInternalServerError)
			return
		}
	}


	// Если есть desktop_token, то редиректим в desktop flow
	if desktopToken, ok := keycloakStateStore[state]; ok && desktopToken != "" {
		serverToken, appErr := c.App.GenerateAndSaveDesktopToken(model.GetMillis(), user)
		if appErr != nil {
			http.Error(w, "Desktop token error: "+appErr.Error(), http.StatusInternalServerError)
			return
		}

		query := map[string]string{
			"client_token": desktopToken,
			"server_token": *serverToken,
		}
		if strings.HasPrefix(desktopToken, "dev-") {
			query["isDesktopDev"] = "true"
		}
		redirectURL := utils.AppendQueryParamsToURL(c.GetSiteURLHeader()+"/login/desktop", query)
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Иначе обычный OAuth логин
	session := &model.Session{
		UserId:  user.Id,
		Roles:   user.Roles,
		IsOAuth: true,
		Props: map[string]string{
			"auth_service":  "keycloak",
			"auth_provider": "openid",
			"email":         user.Email,
		},
		ExpiresAt: model.GetMillis() + model.SessionUserAccessTokenExpiryHours*60*60*24,
	}

	session, appErr = c.App.CreateSession(c.AppContext, session)
	if appErr != nil {
		http.Error(w, "Session error: "+appErr.Error(), http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     model.SessionCookieToken,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
	})
	http.SetCookie(w, &http.Cookie{
		Name:  model.SessionCookieUser,
		Value: user.Id,
		Path:  "/",
	})

	http.Redirect(w, r, "/", http.StatusFound)
}


func generateUsername(email string) string {
	at := strings.Index(email, "@")
	username := email
	if at > 0 {
		username = email[:at]
	}
	// только допустимые символы
	re := regexp.MustCompile(`[^a-z0-9._-]`)
	username = strings.ToLower(username)
	username = re.ReplaceAllString(username, "")

	// Обрезаем до 22 символов, если длиннее
	if len(username) > 22 {
		username = username[:22]
	}

	// Убеждаемся, что начинается с буквы
	if len(username) == 0 || username[0] < 'a' || username[0] > 'z' {
		username = "u" + username
	}
	return username
}


func (api *API) InitKeycloakOIDCLocal() {
	// Инициализация клиента

	issuer := os.Getenv("KEYCLOAK_ISSUER")
	clientID := os.Getenv("KEYCLOAK_CLIENT_ID")
	clientSecret := os.Getenv("KEYCLOAK_CLIENT_SECRET")
	redirectURL := os.Getenv("KEYCLOAK_REDIRECT_URL")


	
	provider, err := oidc.NewProvider(context.Background(), issuer)
	if err != nil {
		log.Println(err)
		return
	}

	keycloakVerifier = provider.Verifier(&oidc.Config{ClientID: clientID})
	keycloakOAuth2Config = oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}

	// Роутинг
	api.BaseRoutes.KeyCloak.Handle("/start", api.APIHandlerTrustRequester(api.keycloakLoginStart)).Methods(http.MethodGet)
	api.BaseRoutes.KeyCloak.Handle("/complete", api.APIHandlerTrustRequester(api.keycloakLoginComplete)).Methods(http.MethodGet)
}
