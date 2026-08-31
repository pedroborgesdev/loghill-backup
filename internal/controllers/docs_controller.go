package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func (a *APIController) OpenAPISpec(c *gin.Context) {
	c.File("./docs/openapi.yaml")
}

func (a *APIController) DocsRedirect(c *gin.Context) {
	c.Redirect(http.StatusTemporaryRedirect, "/docs/index.html")
}

func (a *APIController) Docs(c *gin.Context) {
	ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.URL("/openapi.yaml"),
		ginSwagger.PersistAuthorization(true),
	)(c)
}
