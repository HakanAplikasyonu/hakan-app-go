export { LegalPublicPage } from "./components/legal-public-page";
export { WhatsAppExtensionPrivacyPage } from "./components/whatsapp-extension-privacy-page";
export { LegalMarkdown } from "./components/legal-markdown";
export { LegalDraftNotice } from "./components/legal-draft-notice";
export {
  LegalDocumentBody,
  LegalDocumentMeta,
} from "./components/legal-document-view";
export { LegalDocumentDialog } from "./components/legal-document-dialog";
export { LegalConsentCheckbox } from "./components/legal-consent-checkbox";
export { OfficeDpaCard } from "./components/office-dpa-card";
export { MyLegalAcceptancesCard } from "./components/my-legal-acceptances-card";
export {
  useLegalConsent,
  type LegalConsentState,
} from "./hooks/use-legal-consent";
export * from "./hooks/use-legal";
export {
  isLegalVersionOutdated,
  legalDocumentService,
  legalKeys,
  LegalVersionOutdatedError,
  withLegalVersionCheck,
  type LegalDocument,
  type LegalKind,
  type MyLegalAcceptance,
  type OfficeDPAStatus,
} from "./services/legal.service";
