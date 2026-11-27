-- compat: previous
-- «Общий доступ → Все, у кого есть ссылка» (HRN-1791, docs/hub/design-access.md, docs/hub/resource-access.md): the one link of an object that opens it for
-- anyone, Google's general access. It is a guest link like the others, marked general = 1, never listed among them and
-- kept when switched off, so switching it on again gives the same address. resource_guest_links keeps the sealed token
-- of its general link (token_enc) to copy the address again. Additive only: the previous binary neither reads nor
-- writes these columns and serves a general link as the guest link it is.
ALTER TABLE design_share_links ADD COLUMN general INTEGER;
ALTER TABLE resource_guest_links ADD COLUMN general INTEGER;
ALTER TABLE resource_guest_links ADD COLUMN token_enc TEXT;
