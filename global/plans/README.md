# Global Plans

`catalog.json` is the source of truth for Hivy billing plans.

- `visible: true` plans are returned by `GET /v1/plans` and can be selected for new checkout.
- `visible: false` plans stay in the database so existing subscribed orgs can keep seeing and renewing them.
- `active: false` removes the plan from the active billing catalog; prefer `visible: false` when retiring a plan without breaking existing subscriptions.
- `price_cents` stores the minor currency unit. For NGN, `4990000` means `NGN 49,900`.
- `paystack` metadata is kept here so a Paystack sync command can use the same catalog instead of a separate plan list.
