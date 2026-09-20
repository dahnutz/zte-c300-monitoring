export type ONUSample = {
  time: string;
  device_id: string;
  serial_number: string;
  board: number;
  pon: number;
  onu_id: number;
  name: string;
  onu_type: string;
  status: string;
  rx_power: number | null;
  tx_power: number | null;
  eth_status?: string;
  eth_link_state?: string;
  eth_admin_state?: string;
  eth_speed_mbps?: number | null;
  eth_ports?: EthPort[];
  status_changed_at?: string;
  previous_status?: string;
  expected_eth_ports?: number;
};

export type EthPort = {
  port: number;
  admin: string;
  link: string;
  speed_mbps?: number | null;
  duplex?: string;
};

export type CollectionRun = {
  id: number;
  device_id: string;
  started_at: string;
  finished_at?: string;
  status: string;
  pons_ok: number;
  pons_error: number;
  onus_sampled: number;
  duration_ms?: number;
  error?: string;
};

export type CountResult = {
  group_by: string;
  run_id?: number;
  from: string;
  to: string;
  total: number;
  counts: { key: string; count: number }[];
};

export type Envelope<T> = {
  code: number;
  status: string;
  data: T;
  message?: string;
};

// UI types are a subset of the API JSON. Extra handler fields (device_id,
// board, pon, onu_id, gone_at, discovery_status) are present on the wire
// and ignored here until a page needs them.
export type StatusEvent = {
  time: string;
  serial_number: string;
  status: string;
  previous_status?: string;
  source?: string;
};

export type EthEvent = {
  time: string;
  serial_number: string;
  port: number;
  link_state: string;
  admin_state: string;
  previous_link?: string;
  previous_admin?: string;
  speed_mbps?: number | null;
};

export type UnauthONU = {
  serial_number: string;
  board?: number;
  pon?: number;
  onu_type?: string;
  first_seen_at: string;
  last_seen_at: string;
};

export type UnauthList = {
  status: string;
  oid?: string;
  message?: string;
  observed_at: string;
  count: number;
  onus: UnauthONU[];
};

export type LiveOnu = {
  board: number;
  pon: number;
  onu_id: number;
  name: string;
  onu_type: string;
  serial_number: string;
  rx_power: string;
  tx_power: string;
  status: string;
  observed_at?: string;
  ethernet: {
    status: string;
    observed_at?: string;
    ports: {
      port_index: number;
      admin_state: string;
      link_state: string;
      speed_mbps?: number | null;
      duplex?: string;
    }[];
  };
};
