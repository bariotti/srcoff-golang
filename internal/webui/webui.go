// Package webui contém a interface web (SPA de página única) embutida no binário
// via go:embed, para que o executável seja autocontido (não depende da pasta de
// templates em disco nem do diretório de trabalho).
package webui

import _ "embed"

//go:embed app.html
var appHTML string

// AppHTML devolve o conteúdo do app.html embutido no binário.
func AppHTML() string { return appHTML }
