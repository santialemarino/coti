// The ARCA pages the connection guide sends an administrator to, for each environment.
export const ARCA_URLS = {
  login: 'https://auth.afip.gob.ar/contribuyente_/login.xhtml',
  authorizationGuide: {
    HOMOLOGATION: 'https://www.arca.gob.ar/ws/WSASS/html/crearautorizacion.html',
    PRODUCTION: 'https://www.afip.gob.ar/ws/WSAA/ADMINREL.DelegarWS.pdf',
  },
  certificateGuide: 'https://www.afip.gob.ar/ws/WSAA/wsaa_obtener_certificado_produccion.pdf',
} as const;
