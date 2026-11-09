// node examples/balance.ts
import { Account } from '@john-antibot/sdk'

const account = new Account({ apiKey: process.env.JOHNANTIBOT_API_KEY })
const { metered, balanceUsd } = await account.balance()
console.log(metered ? `$${balanceUsd.toFixed(2)}` : 'this key is not metered')
