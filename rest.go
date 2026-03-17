package main

import (
	"fmt"
	"net/http"

	"github.com/ghibranalj/signifer/db/sqlc"
	"github.com/ghibranalj/signifer/ui"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

type Rest struct {
	repo     *sqlc.Queries
	User     string
	Password string
}

func (r *Rest) basicAuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		user, password, ok := c.Request().BasicAuth()
		if !ok || user != r.User || password != r.Password {
			c.Response().Header().Set("WWW-Authenticate", `Basic realm="Signifer"`)
			return c.NoContent(http.StatusUnauthorized)
		}
		return next(c)
	}
}

func (r *Rest) Start(port int) error {
	e := echo.New()

	// Apply basic auth middleware to all routes
	e.Use(r.basicAuthMiddleware)

	r.RegisterRoutes(e)

	return e.Start(fmt.Sprintf(":%d", port))
}

func (r *Rest) RegisterRoutes(e *echo.Echo) {
	// List devices
	e.GET("/", r.listDevices)
	e.GET("/devices", r.listDevices)

	// Add device
	e.GET("/devices/new", r.showAddDevice)
	e.POST("/devices", r.createDevice)

	// Edit device
	e.GET("/devices/:id/edit", r.showEditDevice)
	e.POST("/devices/:id", r.updateDevice)

	// Delete device
	// NOTE: HTML forms only support GET and POST, so we use POST for delete
	e.GET("/devices/:id/delete", r.showDeleteDevice)
	e.POST("/devices/:id/delete", r.deleteDevice)
}

func (r *Rest) listDevices(c *echo.Context) error {
	devices, err := r.repo.GetDevices(c.Request().Context())
	if err != nil {
		return err
	}
	return ui.Render(c, ui.DeviceList(devices))
}

func (r *Rest) showAddDevice(c *echo.Context) error {
	return ui.Render(c, ui.DeviceAddPage())
}

func (r *Rest) createDevice(c *echo.Context) error {
	deviceName := c.FormValue("device_name")
	hostname := c.FormValue("hostname")
	if deviceName == "" || hostname == "" {
		return c.Redirect(http.StatusFound, "/devices/new")
	}

	id := uuid.New().String()
	params := sqlc.CreateDevicesParams{
		ID:         id,
		DeviceName: deviceName,
		Hostname:   hostname,
	}

	_, err := r.repo.CreateDevices(c.Request().Context(), params)
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusFound, "/")
}

func (r *Rest) showEditDevice(c *echo.Context) error {
	id := c.Param("id")
	devices, err := r.repo.GetDevices(c.Request().Context())
	if err != nil {
		return err
	}

	for _, device := range devices {
		if fmt.Sprintf("%s", device.ID) == id {
			return ui.Render(c, ui.DeviceEditPage(device))
		}
	}

	return c.Redirect(http.StatusFound, "/")
}

func (r *Rest) updateDevice(c *echo.Context) error {
	id := c.Param("id")
	deviceName := c.FormValue("device_name")
	hostname := c.FormValue("hostname")
	if deviceName == "" || hostname == "" {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/devices/%s/edit", id))
	}

	params := sqlc.UpdateDevicesParams{
		ID:         id,
		DeviceName: deviceName,
		Hostname:   hostname,
	}

	_, err := r.repo.UpdateDevices(c.Request().Context(), params)
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusFound, "/")
}

func (r *Rest) showDeleteDevice(c *echo.Context) error {
	id := c.Param("id")
	devices, err := r.repo.GetDevices(c.Request().Context())
	if err != nil {
		return err
	}

	for _, device := range devices {
		if fmt.Sprintf("%s", device.ID) == id {
			return ui.Render(c, ui.DeviceDeletePage(device))
		}
	}

	return c.Redirect(http.StatusFound, "/")
}

func (r *Rest) deleteDevice(c *echo.Context) error {
	id := c.Param("id")

	_, err := r.repo.DeleteDevice(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.Redirect(http.StatusFound, "/")
}
