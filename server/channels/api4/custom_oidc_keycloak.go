package api4

import (
	"context"
	"log"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/mattermost/mattermost/server/public/model"

	"golang.org/x/oauth2"
)

var (
	keycloakVerifier     *oidc.IDTokenVerifier
	keycloakOAuth2Config oauth2.Config
)

func (api *API) keycloakLoginStart(c *Context, w http.ResponseWriter, r *http.Request) {
	state := "mattermost" // TODO: csrf
	url := keycloakOAuth2Config.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusFound)
}

func (api *API) keycloakLoginComplete(c *Context, w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	code := r.URL.Query().Get("code")

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
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Invalid claims", http.StatusInternalServerError)
		return
	}

	user, appErr := c.App.GetUserByEmail(claims.Email)
	if appErr != nil {
		// create user
		user = &model.User{
			Email:    claims.Email,
			Username: generateUsername(claims.Email),
			Password: model.NewId(),
		}
		user, appErr = c.App.CreateUser(c.AppContext, user)
		if appErr != nil {
			http.Error(w, "User create error: "+appErr.Error(), http.StatusInternalServerError)
			return
		}
	}

	session := &model.Session{
		UserId:  user.Id,
		Roles:   user.Roles, // Наследуем роли пользователя
		IsOAuth: true,       // Указываем, что вход по OAuth
		Props: map[string]string{
			"auth_service":  "keycloak",
			"auth_provider": "openid",
			"email":         user.Email,
		},
		DeviceId:  "",                                                                   // если есть, можно получить из запроса
		ExpiresAt: model.GetMillis() + model.SessionUserAccessTokenExpiryHours*60*60*24, // срок действия по дефолту
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
	if at := len(email); at > 0 {
		return email[:at]
	}
	return "user"
}

func (api *API) InitKeycloakOIDCLocal() {
	// Инициализация клиента
	issuer := "http://192.168.145.64:8181/realms/master"
	clientID := "mattermost-client"
	clientSecret := "d5Nyey9Y8fIp8lEW4DmZ5cZitnsqUSEV"
	redirectURL := "http://192.168.145.35:8065/api/v4/auth/keycloak/complete"

	
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
