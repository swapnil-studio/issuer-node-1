export type RouteID =
  | "connectionDetails"
  | "connections"
  | "credentialDetails"
  | "credentials"
  | "importSchema"
  | "issueCredential"
  | "createAuthCredential"
  | "issuerState"
  | "linkDetails"
  | "notFound"
  | "schemaDetails"
  | "schemas"
  | "identities"
  | "createIdentity"
  | "identityDetails"
  | "onboarding"
  | "displayMethods"
  | "displayMethodDetails"
  | "createDisplayMethod"
  | "keyDetails"
  | "createKey"
  | "createPaymentOption"
  | "paymentOptions"
  | "paymentOptionDetails"
  | "paymentRequestDetils"
  | "paymentRequests";

export type Layout = "fullWidth" | "fullWidthGrey" | "sider";

type Routes = Record<
  RouteID,
  {
    layout: Layout;
    path: string;
  }
>;

export const ROUTES: Routes = {
  connectionDetails: {
    layout: "sider",
    path: "/connections/:connectionID",
  },
  connections: {
    layout: "sider",
    path: "/connections",
  },
  createAuthCredential: {
    layout: "sider",
    path: "/credentials/auth",
  },
  createDisplayMethod: {
    layout: "sider",
    path: "/display-methods/create",
  },
  createIdentity: {
    layout: "sider",
    path: "/identities/create",
  },
  createKey: {
    layout: "sider",
    path: "/keys/create",
  },
  createPaymentOption: {
    layout: "sider",
    path: "/payments/options/create",
  },
  credentialDetails: {
    layout: "sider",
    path: "/credentials/issued/:credentialID",
  },
  credentials: {
    layout: "sider",
    path: "/credentials/:tabID",
  },
  displayMethodDetails: {
    layout: "sider",
    path: "/display-methods/:displayMethodID",
  },
  displayMethods: {
    layout: "sider",
    path: "/display-methods",
  },
  identities: {
    layout: "sider",
    path: "/identities",
  },
  identityDetails: {
    layout: "sider",
    path: "/identities/:identityID",
  },
  importSchema: {
    layout: "sider",
    path: "/schemas/import-schema",
  },
  issueCredential: {
    layout: "sider",
    path: "/credentials/issue",
  },
  issuerState: {
    layout: "sider",
    path: "/issuer-state",
  },
  keyDetails: {
    layout: "sider",
    path: "/keys/:keyID",
  },
  linkDetails: {
    layout: "sider",
    path: "/credentials/links/:linkID",
  },
  notFound: {
    layout: "fullWidth",
    path: "/*",
  },
  onboarding: {
    layout: "fullWidthGrey",
    path: "/onboarding",
  },
  paymentOptionDetails: {
    layout: "sider",
    path: "/payments/options/:paymentOptionID",
  },
  paymentOptions: {
    layout: "sider",
    path: "/payments/options",
  },
  paymentRequestDetils: {
    layout: "sider",
    path: "/payments/requests/:paymentRequestID",
  },
  paymentRequests: {
    layout: "sider",
    path: "/payments/requests",
  },
  schemaDetails: {
    layout: "sider",
    path: "/schemas/:schemaID",
  },
  schemas: {
    layout: "sider",
    path: "/schemas",
  },
};
