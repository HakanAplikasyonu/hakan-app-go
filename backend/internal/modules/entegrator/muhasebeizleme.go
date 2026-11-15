package entegrator

// muhasebeizleme (#59) is the BasicIntegration preset of the go-ubltr
// uyumsoft module: importing provider/uyumsoft (uyumsoft.go) registers it
// as "muhasebeizleme". Same read-only surface as Uyumsoft (list, data,
// status); SetInvoicesTaken is not reachable through Client (30 §5).
// Credentials: the API key goes in Credentials.Password (the preset sends
// it in userInfo); Credentials.APIKey may hold the same value. Page size is
// capped at 1000 by the preset; MinPollInterval is 5 minutes.
