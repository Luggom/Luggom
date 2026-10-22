# Add a bank / rail — testnet

> Today only **Viacredi (Brazil)** is proven end to end.
> **The US and Canada** are the next markets. A rail only lights up with **evidence**:
> a **sanitised** real receipt + the payment's **canonical field**.

## What you need

- a real account at the bank (US/CA or another country);
- a **paid** tax/property tax with a receipt;
- the **sanitised** receipt (no name, address, account, payer SSN/EIN/CPF);
- the **canonical field** of the payment (e.g. EIN/BN, receipt ID, PIX E2E in Brazil);
- the bank **domain**; country and currency; the tax entity slug (e.g. `us:county:...`);
- a proposed extractor (receipt field → value).

## How to sanitise

- Remove name, address, account, branch, PIX key, cookies and session headers.
- Keep: bank, country, currency, amount, date, beneficiary and the **canonical field**.
- If the canonical field identifies a person, mask it partially but **keep the format**.
- **Never** publish the real receipt or personal data on Telegram.

## How to submit

1. Open a GitHub issue with the **Rail report** template (`[rail] <bank>`).
2. Attach the sanitised version and the requested fields.
3. Propose the extractor (field → value) and, if possible, a fixture.
4. **Do not** paste the real receipt; the issue is public.

## What the team does

1. Confirms the **canonical field** and the tax nature (a real tax, not a transfer).
2. Adds the provider/domain (item 153 / `MsgAddBankProvider`).
3. Writes the extractor + test.
4. The bank lights up for every customer of that bank.

## US / Canada — notes

- **US:** EIN when the receipt carries it; otherwise resolve by name (county/state), only if
  unambiguous. States by name.
- **Canada:** BN/NEQ when available; CRA / Revenu Québec.
- **Without a canonical field per rail, the rail stays off** (fail-closed).
- The payer **cannot** be a government entity (anti-inflation).
- The receipt must be a **real tax** (not a PIX/transfer to a whitelisted CNPJ/EIN).

## Test kit checklist

- [ ] real **paid** receipt
- [ ] sanitised version (no PII)
- [ ] canonical field identified
- [ ] bank / domain / country / currency
- [ ] tax entity (slug)
- [ ] proposed extractor
- [ ] issue opened (no PII)

Questions: https://t.me/+XRWgwPwvz4hkOTJh (English).
