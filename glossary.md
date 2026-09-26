# CMS Open Payments glossary

Plain words mapped to the local SQLite columns (`openpayments-pp-cli schema --json`) and the CMS source fields they come from. `openpayments-pp-cli ask "<question>"` prints this glossary with worked examples.

| Plain word | Local column | CMS field | Meaning |
|---|---|---|---|
| doctor, clinician, recipient | `npi, recipient_name` | `covered_recipient_npi, covered_recipient_first/last_name` | Covered recipient: physician or non-physician practitioner (NP, PA, CRNA...) identified by NPI. |
| teaching hospital | `teaching_hospital_ccn, teaching_hospital_name` | `teaching_hospital_ccn` | Hospital recipient, keyed by CMS Certification Number (CCN). |
| company, manufacturer, sponsor, payer | `company, company_id` | `applicable_manufacturer_or_applicable_gpo_making_payment_name/_id` | The manufacturer or GPO that made the payment. Research sponsors are this column in payments_research. |
| amount, dollars, paid | `amount (REAL USD); ownership: amount_invested, value_of_interest` | `total_amount_of_payment_usdollars` | Payment value in US dollars. CMS publishes strings; the local store casts to REAL. |
| specialty | `specialty (primary), specialties (all six, '; ' joined)` | `covered_recipient_specialty_1..6` | NUCC taxonomy path, e.g. 'Allopathic & Osteopathic Physicians\|Pain Medicine\|Interventional Pain Medicine'. Match with LIKE '%Pain Medicine%'. |
| nature, type of payment | `nature` | `nature_of_payment_or_transfer_of_value` | Consulting Fee, Food and Beverage, Travel and Lodging, Royalty or License, Compensation for services ... speaker, Education, Grant, Gift, ... |
| speaking, speaker fees | `nature LIKE 'Compensation for%' OR nature='Honoraria'` | `nature_of_payment_or_transfer_of_value` | Faculty/speaker compensation categories. |
| trial, study, NCT | `nct_id, name_of_study` | `clinicaltrials_gov_identifier, name_of_study` | ClinicalTrials.gov ID on research payments (payments_research). |
| principal investigator, PI | `research_investigators.*` | `principal_investigator_1..5_*` | Up to five PIs per research payment, flattened one row per slot. |
| product, drug, device | `products.name, products.kind, products.category` | `name_of_drug_or_biological_or_device_or_medical_supply_1..5` | Products tied to a payment (slots 1-5); kind is Drug, Biological, Device or Medical Supply. |
| year, program year | `program_year` | `program_year` | Calendar year the payment was made (CMS publishes each June for the prior year). |
| date | `payment_date (YYYY-MM-DD)` | `date_of_payment (MM/DD/YYYY)` | Payment date normalized to ISO. |
| state, city, zip | `state, city, zip5` | `recipient_state, recipient_city, recipient_zip_code` | Recipient's primary business address (public record). |
| disputed | `dispute` | `dispute_status_for_publication` | 'Yes' when the recipient disputed the record; always shown, never interpreted. |
| ownership, investment | `payments_ownership` | `Ownership Payment Data` | Physician (or immediate family) ownership and investment interests in a manufacturer or GPO. |
| changed, corrected, deleted | `sync_changes, change_type` | `change_type` | CMS change_type (NEW/CHANGED/UNCHANGED) plus the CLI's per-sync diff. |
| provenance | `record_id, program_year, dataset_id, dataset_modified` | `record_id` | Every payment row carries its CMS record id and the dataset modified date it was synced from. |

## Worked questions

1. **Top 20 PA recipients by total general payments in 2024**
   ```bash
   openpayments-pp-cli top --by recipient --state PA --year 2024 --limit 20
   ```
   SQL: `SELECT npi, MAX(recipient_name) name, SUM(amount) total FROM payments_general WHERE state='PA' AND program_year=2024 GROUP BY npi ORDER BY total DESC LIMIT 20`
2. **Top 10 companies paying PA Pain Medicine doctors, 2019-2025**
   ```bash
   openpayments-pp-cli top --by company --state PA --specialty "Pain Medicine" --year 2019-2025 --limit 10
   ```
   SQL: `SELECT company, SUM(amount) total FROM payments_general WHERE state='PA' AND specialties LIKE '%Pain Medicine%' AND program_year BETWEEN 2019 AND 2025 GROUP BY company ORDER BY total DESC LIMIT 10`
3. **Which companies pay neurosurgeons (or any specialty) the most**
   ```bash
   openpayments-pp-cli top --by company --specialty "Neurological Surgery" --limit 10
   ```
   SQL: `SELECT company, SUM(amount) total FROM payments_general WHERE specialties LIKE '%Neurological Surgery%' GROUP BY company ORDER BY total DESC LIMIT 10`
4. **Every research payment in NJ with an NCT ID, 2024**
   ```bash
   openpayments-pp-cli research --state NJ --year 2024 --has-nct --limit 1000
   ```
   SQL: `SELECT record_id, nct_id, name_of_study, company, amount FROM payments_research WHERE state='NJ' AND program_year=2024 AND nct_id IS NOT NULL`
5. **All PIs paid for a given NCT ID**
   ```bash
   openpayments-pp-cli investigators --nct NCT04280705
   ```
   SQL: `SELECT i.npi, i.name, SUM(p.amount) FROM research_investigators i JOIN payments_research p USING(record_id, program_year) WHERE p.nct_id='NCT04280705' GROUP BY i.npi`
6. **A single doctor's full dossier by NPI**
   ```bash
   openpayments-pp-cli dossier 1234567890
   ```
7. **Doctors in PA new to industry payments in 2025**
   ```bash
   openpayments-pp-cli new-recipients --year 2025 --state PA
   ```
8. **Largest YoY increase for any company-doctor pair in PA**
   ```bash
   openpayments-pp-cli rising --state PA --limit 20
   ```
9. **Stryker's total payments to Orthopaedic Surgeons in PA by year**
   ```bash
   openpayments-pp-cli company stryker --state PA --specialty "Orthopaedic Surgery" --json --select by_year
   ```
   SQL: `SELECT program_year, SUM(amount) FROM payments_general WHERE company LIKE '%Stryker%' AND state='PA' AND specialties LIKE '%Orthopaedic Surgery%' GROUP BY program_year`
10. **Breakdown of a doctor's payments by nature of payment**
   ```bash
   openpayments-pp-cli nature --npi 1234567890
   ```
   SQL: `SELECT nature, SUM(amount) FROM payments_general WHERE npi='1234567890' GROUP BY nature`
11. **Teaching hospitals in PA ranked by research dollars**
   ```bash
   openpayments-pp-cli hospital --state PA --metric research --limit 20
   ```
12. **Physician ownership interests in NJ, grouped by company**
   ```bash
   openpayments-pp-cli ownership --state NJ --group-by company
   ```
13. **Doctors within 25 miles of ZIP 19002 with research payments**
   ```bash
   openpayments-pp-cli near --zip 19002 --miles 25 --type research
   ```
14. **Products (devices) most linked to payments to neurosurgeons in PA**
   ```bash
   openpayments-pp-cli top --by product --state PA --specialty "Neurological Surgery"
   ```
15. **Doctors who received research payments from 3+ sponsors**
   ```bash
   openpayments-pp-cli research-sites --min-sponsors 3
   ```
16. **KOL ranking for Physical Medicine & Rehabilitation in PA**
   ```bash
   openpayments-pp-cli kol --specialty "Physical Medicine" --state PA
   ```
17. **Recruiting spine trials in PA/NJ whose sponsor pays no local PI**
   ```bash
   openpayments-pp-cli trials gaps --condition "spine" --state PA,NJ
   ```
18. **Records changed since the last sync**
   ```bash
   openpayments-pp-cli changed --since-last-sync
   ```
   SQL: `SELECT * FROM sync_changes WHERE sync_run=(SELECT MAX(sync_run) FROM sync_changes)`
19. **Concentration of spend for a named company**
   ```bash
   openpayments-pp-cli concentration "Stryker Corporation"
   ```
20. **Which recipients do two companies both pay?**
   ```bash
   openpayments-pp-cli overlap Stryker "Zimmer Biomet" --state PA
   ```
21. **Custom question: write SQL**
   ```bash
   openpayments-pp-cli schema --json  # then: openpayments-pp-cli sql "SELECT ..."
   ```
