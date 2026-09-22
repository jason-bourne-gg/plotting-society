-- Schema smoke test: the rules that are enforced in Postgres rather than in Go,
-- because they are the ones that must hold even if a handler is wrong.
--
--   make up && make smoke
--
-- Everything runs inside a transaction that is rolled back, so it is safe
-- against a database with real data in it.

BEGIN;
DO $$
DECLARE
  b uuid; u uuid; s uuid; p uuid; f uuid; rid uuid;
  c numeric; d numeric; ok boolean;
  failures int := 0;
BEGIN
  INSERT INTO builders (name, slug) VALUES ('Smoke Builder','smoke-builder-test') RETURNING id INTO b;
  INSERT INTO users (builder_id, email, name, role, password_hash)
    VALUES (b,'smoke-admin@test.invalid','Admin','builder_admin','x') RETURNING id INTO u;
  INSERT INTO societies (builder_id, name, slug) VALUES (b,'Smoke Society','smoke-society-test') RETURNING id INTO s;
  INSERT INTO plots (society_id, plot_no, status) VALUES (s,'1','available') RETURNING id INTO p;

  -- A ledger row is money in or money out, never both.
  BEGIN
    INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, created_by)
      VALUES (s, CURRENT_DATE, 'x', 'both sides', 100, 100, u);
    RAISE NOTICE 'FAIL 1: a both-sides ledger entry was accepted';
    failures := failures + 1;
  EXCEPTION WHEN check_violation THEN
    RAISE NOTICE 'PASS 1: both-sides ledger entry rejected';
  END;

  INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, created_by)
    VALUES (s, CURRENT_DATE, 'roads', 'tar patch', 0, 5000, u) RETURNING id INTO f;

  -- A reversal is the mirror image of the original, linked back to it.
  INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, reverses_id, created_by)
  SELECT society_id, CURRENT_DATE, head, 'Reversal: wrong head', debit, credit, id, u
    FROM fund_entries WHERE id = f AND reverses_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM fund_entries r WHERE r.reverses_id = fund_entries.id)
  RETURNING id, credit, debit INTO rid, c, d;
  IF rid IS NOT NULL AND c = 5000 AND d = 0 THEN
    RAISE NOTICE 'PASS 2: reversal mirrored a 5000 debit into a 5000 credit';
  ELSE
    RAISE NOTICE 'FAIL 2: reversal wrong (credit=% debit=%)', c, d;
    failures := failures + 1;
  END IF;

  -- Reversing the same entry twice must match nothing.
  rid := NULL;
  INSERT INTO fund_entries (society_id, entry_date, head, description, credit, debit, reverses_id, created_by)
  SELECT society_id, CURRENT_DATE, head, 'again', debit, credit, id, u
    FROM fund_entries WHERE id = f AND reverses_id IS NULL
      AND NOT EXISTS (SELECT 1 FROM fund_entries r WHERE r.reverses_id = fund_entries.id)
  RETURNING id INTO rid;
  IF rid IS NULL THEN
    RAISE NOTICE 'PASS 3: reversing the same entry twice matched no rows';
  ELSE
    RAISE NOTICE 'FAIL 3: the entry was reversed twice';
    failures := failures + 1;
  END IF;

  SELECT sum(credit)-sum(debit) INTO c FROM fund_entries WHERE society_id = s;
  IF c = 0 THEN
    RAISE NOTICE 'PASS 4: ledger nets to zero after the reversal';
  ELSE
    RAISE NOTICE 'FAIL 4: balance is %', c;
    failures := failures + 1;
  END IF;

  -- The breach expression the inbox sorts on.
  INSERT INTO queries (society_id, plot_id, raised_by, category, subject, sla_due_at)
    VALUES (s, p, u, 'documents_legal', 'late one', now() - interval '1 day');
  SELECT (resolved_at IS NULL AND sla_due_at < now()) INTO ok FROM queries WHERE society_id = s;
  IF ok THEN
    RAISE NOTICE 'PASS 5: an overdue query reads as breached';
  ELSE
    RAISE NOTICE 'FAIL 5: breach expression wrong';
    failures := failures + 1;
  END IF;

  -- Staff belong to a builder; owners do not.
  BEGIN
    INSERT INTO users (email, name, role, password_hash)
      VALUES ('smoke-staff@test.invalid','Staff','builder_staff','x');
    RAISE NOTICE 'FAIL 6: builder_staff without a builder was accepted';
    failures := failures + 1;
  EXCEPTION WHEN check_violation THEN
    RAISE NOTICE 'PASS 6: builder_staff without a builder rejected';
  END;

  INSERT INTO users (email, name, role, password_hash)
    VALUES ('smoke-owner@test.invalid','Owner','owner','x');
  RAISE NOTICE 'PASS 7: owner without a builder accepted';

  -- A guest leaves a lead without any account existing.
  INSERT INTO enquiries (society_id, plot_id, name, phone, message, source)
    VALUES (s, p, 'Guest', '9822000000', 'is this plot open?', 'plot_detail');
  RAISE NOTICE 'PASS 8: guest enquiry stored with no account';

  BEGIN
    INSERT INTO plots (society_id, plot_no, status) VALUES (s,'1','available');
    RAISE NOTICE 'FAIL 9: a duplicate plot number was accepted';
    failures := failures + 1;
  EXCEPTION WHEN unique_violation THEN
    RAISE NOTICE 'PASS 9: duplicate plot number in one society rejected';
  END;

  IF failures > 0 THEN
    RAISE EXCEPTION '% smoke test(s) failed', failures;
  END IF;
  RAISE NOTICE 'all schema smoke tests passed';
END $$;
ROLLBACK;
