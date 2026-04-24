// Shared SSL CA options used by both MQTT and WebSocket provision forms.
// The Shelly Gen2+ API accepts exactly these four values for *.ssl_ca.
export const sslCAOptions = [
  { value: '', label: 'None (no TLS)', description: 'Plain TCP — TLS disabled' },
  {
    value: '*',
    label: '* (skip validation)',
    description: 'TLS but do not validate certificate',
  },
  { value: 'ca.pem', label: 'ca.pem', description: 'Built-in CA bundle' },
  { value: 'user_ca.pem', label: 'user_ca.pem', description: 'User-uploaded CA certificate' },
];
