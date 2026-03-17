package ui

import (
	"fmt"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"
)

func Render(c *echo.Context, view templ.Component) error {

	return view.Render(c.Request().Context(), c.Response())
}

func RenderPage(view templ.Component) echo.HandlerFunc {
	return func(c *echo.Context) error {
		return Render(c, view)
	}
}

func urlF(format string, a ...any) templ.SafeURL {
	s := fmt.Sprintf(format, a...)

	return templ.SafeURL(s)
}
