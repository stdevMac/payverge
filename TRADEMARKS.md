# Trademarks

The Payverge source code is licensed under the Apache License 2.0 (see
`LICENSE`). Section 6 of that licence says it "does not grant permission to
use the trade names, trademarks, service marks, or product names of the
Licensor". This file explains what that means for the Payverge name and logo.

## What the marks are

"Payverge", the Payverge logo and the Payverge favicons and app icons are
marks of Marcos Maceo. These files carry the logo and icons:

- `frontend/public/images/PayvergeLogo.*` and `frontend/public/images/logo.*`
- `frontend/public/favicon*`, `apple-touch-icon.png`, `android-chrome-*`,
  `maskable-icon-*`
- `site/assets/logo.svg`, `favicon.ico`, `apple-touch-icon.png` and
  `og-image.png` (the payverge.io landing page)

The files are in the repository so the project builds and runs as it is.
That does not license the marks.

## Allowed without asking

- Nominative use. You can say that your software is "based on Payverge", is
  "a fork of Payverge", or "works with Payverge", or describe Payverge
  accurately in articles, talks, comparisons and documentation.
- Running an unmodified or modified copy for yourself or your own
  organisation, including for evaluation and internal use.
- Linking to the upstream project and keeping the copyright and `NOTICE`
  attributions that the Apache License requires.

## Needs a rebrand

If you offer a fork, or a modified build, to other people as a hosted service
or as a distributed product, give it your own name and logo. That way users
will not mistake it for the upstream project or for a service its authors
run. Set the brand with these environment variables at deploy time:

| Setting | Default | Set it to |
| --- | --- | --- |
| `PRODUCT_NAME` | `Payverge` | your product's name |
| `COMPANY_NAME` | the value of `PRODUCT_NAME` | the name of whoever operates the service |
| `LEGAL_ENTITY` | `COMPANY_NAME`, if set | the legal name shown in footers and terms |
| `LOGO_URL` | the bundled Payverge logo | an `https://` URL or a root-relative path to your logo |
| `BRAND_COLOR` | `#1a6b6a` | your primary colour (optional; the colour is not a mark) |
| `SUPPORT_EMAIL` | unset (no contact shown) | your support address |
| `SECURITY_EMAIL` | the value of `SUPPORT_EMAIL` | your security contact |

The backend reads these settings. They brand the email it sends and the
instance details it serves at `/api/v1/instance`. Some text and images are
built into the web app, and the settings do not reach all of them. Replace
the favicons, app icons and social cards listed above with your own, along
with the web manifests in `frontend/public/site*.webmanifest` and any
"Payverge" left in the page metadata and the message files under
`frontend/src/i18n/`.

Keep the `LICENSE` and `NOTICE` files and the attributions in them.
Rebranding changes the name, not the authorship.

Do not use "Payverge", or a confusingly similar name or logo, as the name of
your product, company, domain or app-store listing. Do not suggest that the
Payverge authors endorse or operate your service.

## Third-party marks

The names and logos of payment providers, wallets, chains, delivery
platforms and other services in this repository belong to their owners. That
includes `frontend/public/images/plugins/` and
`frontend/public/images/provider-icons/`. They appear only to identify an
integration. The Apache License does not cover them, and their presence does
not mean those companies endorse Payverge. Follow each owner's brand
guidelines when you use them.

## Questions

For permission beyond what this page allows, open an issue on the upstream
repository.
