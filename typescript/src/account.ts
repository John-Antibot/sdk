import { Transport, type CallOptions, type ClientOptions } from './http.ts'

/** What the account of a key has to spend. */
export interface Balance {
  /** The key is charged. A key that is not metered has no balance. */
  metered: boolean
  /** In millionths of a dollar, the unit of a price in the catalog (2000 is $0.002). */
  balanceMicros: number
  /** In dollars, for display. */
  balanceUsd: number
}

/** The account of your key. */
export class Account {
  readonly #http: Transport

  constructor(options: ClientOptions = {}) {
    this.#http = new Transport('account', options, 20_000)
  }

  /** The API base URL. */
  get endpoint(): string {
    return this.#http.endpoint
  }

  /** The balance each paid call takes its price from. Free. */
  async balance(options: CallOptions = {}): Promise<Balance> {
    const b = await this.#http.call<{ metered?: boolean; balanceMicros?: number }>('GET', '/v1/balance', undefined, options.signal)
    const micros = b.balanceMicros ?? 0
    return { metered: b.metered === true, balanceMicros: micros, balanceUsd: micros / 1e6 }
  }
}
