import http from 'k6/http';
import exec from 'k6/execution';
import { check, sleep } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
const batchSize = Number(__ENV.BATCH_SIZE || 500);
const hotAccountCount = Number(__ENV.HOT_ACCOUNT_COUNT || 100);
const rate = Number(__ENV.RATE || 200);
const duration = __ENV.DURATION || '30m';
const preAllocatedVUs = Number(__ENV.PREALLOCATED_VUS || 100);
const maxVUs = Number(__ENV.MAX_VUS || 500);

export const options = {
  scenarios: {
    soak: {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

function makeBatch(size, hotAccounts, iteration) {
  const txs = [];
  const occurredAt = new Date().toISOString();

  for (let i = 0; i < size; i++) {
    txs.push({
      idempotency_key: `idem-${exec.vu.idInTest}-${iteration}-${i}`,
      tenant_id: `tenant-${Math.floor(i / hotAccounts) % 10}`,
      account_id: `acct-${i % hotAccounts}`,
      type: i % 2 === 0 ? 'debit' : 'credit',
      amount: {
        currency: 'USD',
        value: ((i % 5000) + 1).toFixed(2),
      },
      reference: `invoice-${iteration}-${i}`,
      occurred_at: occurredAt,
      metadata: {
        source_channel: 'load-test',
      },
    });
  }

  return JSON.stringify({
    batch_id: `batch-${exec.vu.idInTest}-${iteration}`,
    source: 'k6-soak',
    transactions: txs,
  });
}

export default function () {
  const payload = makeBatch(batchSize, hotAccountCount, exec.scenario.iterationInTest);

  const res = http.post(`${baseUrl}/api/v1/transactions`, payload, {
    headers: { 'Content-Type': 'application/json' },
    timeout: '2s',
  });

  check(res, {
    accepted: (r) => r.status === 202,
  });

  sleep(0.1);
}
