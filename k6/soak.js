import http from 'k6/http';
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

function makeBatch(size, hotAccounts) {
  const txs = [];

  for (let i = 0; i < size; i++) {
    txs.push({
      id: `${__VU}-${__ITER}-${i}`,
      account_id: `acct-${i % hotAccounts}`,
      amount_cents: (i % 5000) + 1,
      currency: 'USD',
      ts: new Date().toISOString(),
    });
  }

  return JSON.stringify(txs);
}

const payload = makeBatch(batchSize, hotAccountCount);

export default function () {
  const res = http.post(`${baseUrl}/api/v1/transactions`, payload, {
    headers: { 'Content-Type': 'application/json' },
    timeout: '2s',
  });

  check(res, {
    accepted: (r) => r.status === 202,
  });

  sleep(0.1);
}
