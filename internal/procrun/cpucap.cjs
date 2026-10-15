'use strict';

const os = require('os');
const n = Number(process.env.GIT_HOOKS_CPUS);

if (Number.isSafeInteger(n) && n > 0) {
  const originalCPUs = os.cpus.bind(os);
  os.availableParallelism = () => n;
  os.cpus = () => {
    const cpus = originalCPUs().slice(0, n);
    // Some systems omit CPU details even though workers can still run.
    const fallback = cpus[0] || {
      model: 'unknown', speed: 0,
      times: { user: 0, nice: 0, sys: 0, idle: 0, irq: 0 },
    };
    while (cpus.length < n) cpus.push(fallback);
    return cpus;
  };
}
