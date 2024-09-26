package frontend

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/usememos/memos/internal/util"
	"github.com/usememos/memos/server/profile"
	"github.com/usememos/memos/store"
)

//go:embed dist
var embeddedFiles embed.FS

type FrontendService struct {
	Profile *profile.Profile
	Store   *store.Store
}

func NewFrontendService(profile *profile.Profile, store *store.Store) *FrontendService {
	return &FrontendService{
		Profile: profile,
		Store:   store,
	}
}

func (s *FrontendService) Serve(_ context.Context, e *echo.Echo) {
	skipper := func(c echo.Context) bool {
		return util.HasPrefixes(c.Path(), "/api", "/memos.api.v1", "/robots.txt", "/sitemap.xml")
	}

	// Use echo static middleware to serve the built dist folder.
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		HTML5:      true,
		Filesystem: getFileSystem("dist"),
		Skipper:    skipper,
	}))
	// Use echo gzip middleware to compress the response.
	// Reference: https://echo.labstack.com/docs/middleware/gzip
	e.Group("assets").Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 5,
	}), func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set(echo.HeaderCacheControl, "max-age=31536000, immutable")
			return next(c)
		}
	}, middleware.StaticWithConfig(middleware.StaticConfig{
		Filesystem: getFileSystem("dist/assets"),
	}))

	// Add routes for robots.txt and sitemap.xml
	s.registerSEORoutes(e)
}

func (s *FrontendService) registerSEORoutes(e *echo.Echo) {
	e.GET("/robots.txt", func(c echo.Context) error {
		if s.Profile == nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Profile is not set")
		}
		instanceURL := s.Profile.InstanceURL
		if instanceURL == "" {
			return echo.NewHTTPError(http.StatusInternalServerError, "Instance URL is not set in profile")
		}
		robotsTxt := fmt.Sprintf(`User-agent: *
Allow: /
Host: %s
Sitemap: %s/sitemap.xml`, instanceURL, instanceURL)
		return c.String(http.StatusOK, robotsTxt)
	})

	e.GET("/sitemap.xml", func(c echo.Context) error {
		ctx := c.Request().Context()
		if s.Profile == nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Profile is not set")
		}
		instanceURL := s.Profile.InstanceURL
		if instanceURL == "" {
			return echo.NewHTTPError(http.StatusInternalServerError, "Instance URL is not set in profile")
		}
		var urlsets []string
		// Append memo list.
		memoList, err := s.Store.ListMemos(ctx, &store.FindMemo{
			VisibilityList: []store.Visibility{store.Public},
		})
		if err != nil {
			return err
		}
		for _, memo := range memoList {
			urlsets = append(urlsets, fmt.Sprintf(`<url><loc>%s</loc></url>`, fmt.Sprintf("%s/m/%d", instanceURL, memo.ID)))
		}
		// Append user list.
		userList, err := s.Store.ListUsers(ctx, &store.FindUser{})
		if err != nil {
			return err
		}
		for _, user := range userList {
			urlsets = append(urlsets, fmt.Sprintf(`<url><loc>%s</loc></url>`, fmt.Sprintf("%s/u/%s", instanceURL, user.Username)))
		}

		sitemap := fmt.Sprintf(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:news="http://www.google.com/schemas/sitemap-news/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml" xmlns:mobile="http://www.google.com/schemas/sitemap-mobile/1.0" xmlns:image="http://www.google.com/schemas/sitemap-image/1.1" xmlns:video="http://www.google.com/schemas/sitemap-video/1.1">%s</urlset>`, strings.Join(urlsets, "\n"))
		return c.XMLBlob(http.StatusOK, []byte(sitemap))
	})
}

func getFileSystem(path string) http.FileSystem {
	fs, err := fs.Sub(embeddedFiles, path)
	if err != nil {
		panic(err)
	}
	return http.FS(fs)
}
