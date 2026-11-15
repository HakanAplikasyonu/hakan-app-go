package journals

// Drivers available to Export. Each registers itself in init()
// (accountingexport.Register); a new program is one more import here.
import (
	_ "github.com/HakanAplikasyonu/hakan-app-go/backend/internal/accountingexport/excelformula"
)
