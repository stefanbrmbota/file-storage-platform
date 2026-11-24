-- Test podaci za Notifications servis (US-07, T-07.4).
--
-- Tri test korisnika (seed-ana, seed-bojan, seed-ceca) sa obaveštenjima svih
-- tipova iz design 3.5, pročitanim i nepročitanim. Skripta se može pokrenuti
-- više puta: isti ključ (user_id, notification_id) samo prepiše red, pa se
-- obaveštenja ne dupliraju.
--
-- POKRETANJE (iz root-a repoa, dok radi `docker compose up -d`; tabele pravi
-- sam servis pri pokretanju). Fajl se kopira u Cassandra kontejner i pokrece
-- tamo, jer PowerShell 5.1 pri slanju teksta kroz | kvari slova (š, ć...).
-- Iste komande rade u PowerShell-u i Git Bash-u (u Git Bash-u prvo
-- `export MSYS_NO_PATHCONV=1`):
--
--     docker compose cp services/notifications/scripts/seed-notifications.cql cassandra:/tmp/seed.cql
--     docker compose exec cassandra cqlsh -f /tmp/seed.cql
--
-- PROVERA u bazi:
--     docker compose exec cassandra cqlsh -e "SELECT user_id, message, is_read FROM notifications.notifications_by_user;"
--
-- DA IH VIDIŠ KAO SVOJ NALOG (kroz gateway ili klijent): endpoint vraca samo
-- obavestenja prijavljenog korisnika, pa seed-ana zameni svojim ID-jem (polje
-- "sub" u JWT-u koji dobiješ posle prijave, ili _id u Mongo kolekciji users).
-- Posle kopiranja fajla (prva komanda gore):
--
--     docker compose exec cassandra sh -c "sed 's/seed-ana/TVOJ_ID/g' /tmp/seed.cql | cqlsh"
--
-- BRISANJE test podataka jednog korisnika:
--     docker compose exec cassandra cqlsh -e "DELETE FROM notifications.notifications_by_user WHERE user_id = 'seed-ana'; DELETE FROM notifications.unread_by_user WHERE user_id = 'seed-ana';"
--
-- notification_id je timeuuid sa fiksnim vremenom (minTimeuuid), da bi
-- redosled "od najnovijeg" bio isti pri svakom pokretanju. Svaki red ima
-- default TTL tabele (90 dana od upisa).

USE notifications;

-- seed-ana: 5 obaveštenja, 3 nepročitana -------------------------------------

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-ana', minTimeuuid('2026-11-20 09:15:00+0000'), 'SHARE_GRANTED',
        'Bojan je podelio folder "Fakultet" sa tobom', 'seed-bojan', 'seed-folder-fakultet', 'FOLDER', true);

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-ana', minTimeuuid('2026-11-21 14:40:00+0000'), 'SHARED_FOLDER_UPLOAD',
        'Ceca je otpremila datoteku "Raspored.pdf" u folder "Projekat"', 'seed-ceca', 'seed-file-raspored', 'FILE', true);

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-ana', minTimeuuid('2026-11-22 18:05:00+0000'), 'QUOTA_WARNING',
        'Iskoristila si 90% prostora za skladištenje', '', '', '', false);
INSERT INTO unread_by_user (user_id, notification_id)
VALUES ('seed-ana', minTimeuuid('2026-11-22 18:05:00+0000'));

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-ana', minTimeuuid('2026-11-23 10:30:00+0000'), 'SHARE_REVOKED',
        'Bojan ti je ukinuo pristup folderu "Stari materijali"', 'seed-bojan', 'seed-folder-stari', 'FOLDER', false);
INSERT INTO unread_by_user (user_id, notification_id)
VALUES ('seed-ana', minTimeuuid('2026-11-23 10:30:00+0000'));

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-ana', minTimeuuid('2026-11-24 08:00:00+0000'), 'SHARE_GRANTED',
        'Ceca je podelila datoteku "Ispitna pitanja.docx" sa tobom', 'seed-ceca', 'seed-file-pitanja', 'FILE', false);
INSERT INTO unread_by_user (user_id, notification_id)
VALUES ('seed-ana', minTimeuuid('2026-11-24 08:00:00+0000'));

-- seed-bojan: 2 obaveštenja, 1 nepročitano -----------------------------------

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-bojan', minTimeuuid('2026-11-21 11:00:00+0000'), 'SHARED_FOLDER_UPLOAD',
        'Ana je otpremila datoteku "Beleške.txt" u folder "Fakultet"', 'seed-ana', 'seed-file-beleske', 'FILE', true);

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-bojan', minTimeuuid('2026-11-23 16:20:00+0000'), 'SHARE_GRANTED',
        'Ceca je podelila folder "Projekat" sa tobom', 'seed-ceca', 'seed-folder-projekat', 'FOLDER', false);
INSERT INTO unread_by_user (user_id, notification_id)
VALUES ('seed-bojan', minTimeuuid('2026-11-23 16:20:00+0000'));

-- seed-ceca: 1 pročitano obaveštenje (nema nepročitanih) --------------------

INSERT INTO notifications_by_user (user_id, notification_id, type, message, actor_id, resource_id, resource_type, is_read)
VALUES ('seed-ceca', minTimeuuid('2026-11-22 09:45:00+0000'), 'QUOTA_WARNING',
        'Iskoristila si 90% prostora za skladištenje', '', '', '', true);
