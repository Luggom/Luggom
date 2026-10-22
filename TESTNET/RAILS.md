# Adicionar um banco / rail — testnet

> Hoje só o **Viacredi (Brasil)** está provado de ponta a ponta.
> **EUA e Canadá** são os próximos mercados. Um rail só acende com **evidência**:
> um recibo real **sanitizado** + o **campo canónico** do pagamento.

## O que precisas

- conta real no banco (EUA/CA ou outro país);
- um imposto/property tax **pago**, com comprovante;
- o comprovante **sanitizado** (sem nome, morada, conta, SSN/EIN/CPF do pagador);
- o **campo canónico** do pagamento (ex.: EIN/BN, ID do recibo, PIX E2E no Brasil);
- o **domínio** do banco; país e moeda; o slug da entidade fiscal (ex.: `us:county:...`);
- um extractor proposto (campo do recibo → valor).

## Como sanitizar

- Remove nome, morada, conta, agência, chave PIX, cookies e cabeçalhos de sessão.
- Mantém: banco, país, moeda, valor, data, entidade/beneficiário e o **campo canónico**.
- Se o campo canónico identificar uma pessoa, mascara parcialmente mas **mantém o formato**.
- **Nunca** publiques o recibo real nem dados pessoais no Telegram.

## Como submeter

1. Abre uma issue no GitHub com o template **Rail report** (`[rail] <banco>`).
2. Anexa a versão sanitizada e os campos pedidos.
3. Propõe o extractor (campo → valor) e, se possível, um fixture.
4. **Não** coles o recibo real; a issue é pública.

## O que a equipa faz

1. Confirma o **campo canónico** e a natureza fiscal (imposto real, não uma transferência).
2. Adiciona o provider/domínio (item 153 / `MsgAddBankProvider`).
3. Escreve o extractor + teste.
4. O banco acende para todos os clientes daquele banco.

## EUA / Canadá — notas

- **EUA:** EIN quando o recibo traz; caso contrário, resolução por nome (county/state), só se
  for inequívoca. Estados por nome.
- **Canadá:** BN/NEQ quando disponível; CRA / Revenu Québec.
- **Sem campo canónico por rail, o rail fica desligado** (fail-closed).
- O pagador **não** pode ser entidade governamental (anti-inflação).
- O comprovante tem de ser um **imposto real** (não um PIX/transferência para um CNPJ/EIN
  da whitelist).

## Checklist do test kit

- [ ] recibo real **pago**
- [ ] versão sanitizada (sem PII)
- [ ] campo canónico identificado
- [ ] banco / domínio / país / moeda
- [ ] entidade fiscal (slug)
- [ ] extractor proposto
- [ ] issue aberta (sem PII)

Dúvidas: grupo https://t.me/+XRWgwPwvz4hkOTJh (inglês).
