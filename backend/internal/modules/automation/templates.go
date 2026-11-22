package automation

// Template is a ready-made rule. Every office gets them passive; one click
// turns one on (Ayarlar > Otomasyonlar).
type Template struct {
	Code         string
	Name         string
	TriggerKind  string
	Trigger      Trigger
	OnlyDue      bool
	Channels     []string
	Subject      string
	Body         string
	ApprovalMode string
}

// TemplateForeignInvoices is the monthly "Yabancı faturalarınızı gönderin"
// reminder (#780 KDV2 A2). It is hidden and does not run while the office
// has foreign invoices off (Service.WithGate).
const TemplateForeignInvoices = "yabanci_fatura_1"

// Templates are the ready-made rules (#780 C, Easytrade "SMS Otomasyonu").
var Templates = []Template{
	{
		Code: "muhtasar_26", Name: "Ayın 26'sı muhtasar hatırlatması", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{26}, Hour: 9, Tax: TaxMuhtasar},
		Channels: []string{ChannelSMS},
		Subject:  ":donem muhtasar beyannamesi",
		Body:     "Sayın :mukellef, :donem dönemi muhtasar ve prim hizmet beyannamesinin son günü :son_gun. Bordro ve ödeme belgelerinizi iletmediyseniz bugün iletiniz. :ofis",
	},
	{
		Code: "kdv_28", Name: "Ayın 28'i KDV hatırlatması", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{28}, Hour: 9, Tax: TaxKDV},
		OnlyDue:  true,
		Channels: []string{ChannelSMS, ChannelEmail},
		Subject:  ":donem KDV ödemesi",
		Body:     "Sayın :mukellef, :donem dönemi KDV tahakkukunuz :tutar. Son ödeme günü :son_gun. :ofis",
	},
	{
		Code: "kdv_son_gun_1", Name: "KDV son gününden 1 gün önce", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeBeforeDue, BeforeDueDays: 1, Hour: 10, Tax: TaxKDV},
		OnlyDue:  true,
		Channels: []string{ChannelSMS},
		Subject:  ":donem KDV ödemesi yarın",
		Body:     "Sayın :mukellef, :donem dönemi KDV ödemenizin son günü yarın (:son_gun). Ödenecek tutar :tutar. Ödediyseniz bu mesajı dikkate almayınız. :ofis",
	},
	{
		Code: "yapilandirma_ay_sonu", Name: "Ay sonu yapılandırma taksiti", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{31}, Hour: 10},
		Channels: []string{ChannelSMS},
		Subject:  "Yapılandırma taksiti",
		Body:     "Sayın :mukellef, yapılandırma taksitinizin son ödeme günü bugündür. Ödemeyi yaptıysanız dekontu iletmenizi rica ederiz. :ofis",
	},
	{
		Code: "gecici_ay_1", Name: "Ayın 1'i geçici vergi duyurusu", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{1}, Hour: 9, Tax: TaxGecici},
		Channels: []string{ChannelSMS, ChannelEmail},
		Subject:  ":donem geçici vergi",
		Body:     "Sayın :mukellef, :donem geçici vergi beyannamesinin son günü :son_gun. Dönem belgelerinizi ayın 10'una kadar iletmenizi rica ederiz. :ofis",
	},
	{
		Code: "banka_ekstresi_1_5", Name: "Ayın 1–5'i banka ekstresi", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{1}, Hour: 9},
		Channels: []string{ChannelSMS, ChannelInapp},
		Subject:  ":donem banka ekstreleri",
		Body:     "Sayın :mukellef, :donem dönemine ait banka ekstrelerinizi (:banka) ayın 5'ine kadar yüklemenizi rica ederiz: :link :ofis",
	},
	{
		Code: "kdv_raporu_29", Name: "KDV raporu: ayın 29'u, ofis onayıyla mükellefe", TriggerKind: TriggerCalendar,
		Trigger:      Trigger{Mode: ModeDays, Days: []int{29}, Hour: 9, Tax: TaxKDV, CurrentPeriod: true},
		Channels:     []string{ChannelSMS, ChannelEmail},
		Subject:      ":donem KDV raporu",
		Body:         "Sayın :mukellef, :donem dönemi KDV ön raporunuz hazırlandı. Ayrıntılar ve eksik belgeler için: :link :ofis",
		ApprovalMode: ApprovalDraft,
	},
	{
		Code: TemplateForeignInvoices, Name: "Ayın 1'i yabancı faturalarınızı gönderin", TriggerKind: TriggerCalendar,
		Trigger:  Trigger{Mode: ModeDays, Days: []int{1}, Hour: 9},
		Channels: []string{ChannelInapp, ChannelEmail},
		Subject:  ":donem yabancı hizmet faturaları",
		Body:     "Sayın :mukellef, :donem döneminde yurt dışından aldığınız hizmetlerin (Google Ads, Meta, AWS, yazılım lisansı vb.) faturalarını mükellef panelinden ya da WhatsApp'tan iletmenizi rica ederiz: :link :ofis",
	},
}
