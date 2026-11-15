// Package accountingexport is the accounting-program output layer (docs/02 §3,
// D8): a JournalDraft batch goes in, files for a target program come out.
//
// It follows the database/sql driver pattern. Each driver lives in its own
// package, registers itself in init() with Register, and the application opens
// it by name with Open. Adding Zirve, Mikro or Orka means one new package that
// is imported for its side effect; this package does not change.
//
//	import _ ".../accountingexport/luca"
//	d, err := accountingexport.Open("luca", accountingexport.Config{})
//	issues := d.Validate(ctx, batch)
//	res, err := d.Export(ctx, batch, writer)
//
// Rules shared by every driver (balance, target account plan) live in
// CommonIssues; splitting a batch into files and a long entry into balanced
// pieces is each driver's job (helpers: SplitIntoFiles, SplitEntry). A driver
// only produces files; uploading into a program is a separate interface.
package accountingexport
