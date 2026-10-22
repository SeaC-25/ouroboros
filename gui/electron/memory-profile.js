export const DEFAULT_PACKAGED_MEMORY_MODE = 'low';

const MEMORY_PROFILE_ARGUMENT = '--ouro-desktop-memory-profile=';

const MEMORY_PROFILES = {
  standard: {
    rendererProcessLimit: '3',
    maxOldSpaceSize: 512,
    maxSemiSpaceSize: 16,
    goMemoryLimit: '768MiB',
    goGC: '100',
  },
  balanced: {
    rendererProcessLimit: '2',
    maxOldSpaceSize: 384,
    maxSemiSpaceSize: 12,
    goMemoryLimit: '512MiB',
    goGC: '75',
  },
  low: {
    rendererProcessLimit: '1',
    maxOldSpaceSize: 256,
    maxSemiSpaceSize: 8,
    goMemoryLimit: '320MiB',
    goGC: '50',
  },
};

export function resolveMemoryProfile(requestedMode, isPackaged) {
  const requested = String(requestedMode || '').trim().toLowerCase();
  const mode = Object.hasOwn(MEMORY_PROFILES, requested)
    ? requested
    : isPackaged ? DEFAULT_PACKAGED_MEMORY_MODE : 'standard';
  return { mode, ...MEMORY_PROFILES[mode] };
}

export function applyMemoryEnvironment(environment, profile) {
  environment.OURO_DESKTOP_MEMORY_MODE = profile.mode;
  environment.GOMEMLIMIT = lowerMemoryLimit(environment.GOMEMLIMIT, profile.goMemoryLimit);
  environment.GOGC = lowerGarbageCollectionTarget(environment.GOGC, profile.goGC);
  return environment;
}

function lowerMemoryLimit(current, maximum) {
  const currentBytes = parseMemoryBytes(current);
  const maximumBytes = parseMemoryBytes(maximum);
  if (currentBytes !== null && maximumBytes !== null && currentBytes <= maximumBytes) {
    return String(current);
  }
  return maximum;
}

function lowerGarbageCollectionTarget(current, maximum) {
  const currentTarget = parseNonNegativeInteger(current);
  const maximumTarget = parseNonNegativeInteger(maximum);
  if (currentTarget !== null && maximumTarget !== null && currentTarget <= maximumTarget) {
    return String(current);
  }
  return maximum;
}

function parseMemoryBytes(value) {
  const text = String(value || '').trim();
  const match = text.match(/^(\d+(?:\.\d+)?)\s*(B|KiB|MiB|GiB|TiB)?$/i);
  if (!match) {
    return null;
  }
  const multipliers = {
    b: 1,
    kib: 1024,
    mib: 1024 ** 2,
    gib: 1024 ** 3,
    tib: 1024 ** 4,
  };
  const multiplier = match[2] ? multipliers[match[2].toLowerCase()] : 1;
  const bytes = Number(match[1]) * multiplier;
  return Number.isFinite(bytes) && bytes >= 0 ? bytes : null;
}

function parseNonNegativeInteger(value) {
  const text = String(value || '').trim();
  return /^\d+$/.test(text) ? Number.parseInt(text, 10) : null;
}

export function memoryV8Flags(profile) {
  return `--max-old-space-size=${profile.maxOldSpaceSize} --max-semi-space-size=${profile.maxSemiSpaceSize}`;
}

export function buildMemoryRelaunchArgs(args, profile) {
  const preserved = stripManagedMemoryArguments(args);
  return [
    ...preserved,
    ...expectedMemoryArguments(profile),
  ];
}

export function hasMemoryLaunchProfile(args, profile) {
  const expected = expectedMemoryArguments(profile);
  const actual = managedMemoryArguments(args);
  return actual.length === expected.length && expected.every(argument => actual.includes(argument));
}

function isManagedMemoryArgument(argument) {
  return argument.startsWith(MEMORY_PROFILE_ARGUMENT)
    || argument === '--js-flags'
    || argument.startsWith('--js-flags=')
    || argument === '--renderer-process-limit'
    || argument.startsWith('--renderer-process-limit=')
    || argument === '--max-old-space-size'
    || argument.startsWith('--max-old-space-size=')
    || argument === '--max-semi-space-size'
    || argument.startsWith('--max-semi-space-size=');
}

function expectedMemoryArguments(profile) {
  return [
    `${MEMORY_PROFILE_ARGUMENT}${profile.mode}`,
    `--js-flags=${memoryV8Flags(profile)}`,
    `--renderer-process-limit=${profile.rendererProcessLimit}`,
  ];
}

function managedMemoryArguments(args) {
  const managed = [];
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === '--js-flags' || argument === '--renderer-process-limit'
      || argument === '--max-old-space-size' || argument === '--max-semi-space-size') {
      managed.push(`${argument}=${args[index + 1] ?? ''}`);
      index += 1;
      continue;
    }
    if (isManagedMemoryArgument(argument)) {
      managed.push(argument);
    }
  }
  return managed;
}

function stripManagedMemoryArguments(args) {
  const preserved = [];
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === '--js-flags' || argument === '--renderer-process-limit'
      || argument === '--max-old-space-size' || argument === '--max-semi-space-size') {
      index += 1;
      continue;
    }
    if (!isManagedMemoryArgument(argument)) {
      preserved.push(argument);
    }
  }
  return preserved;
}
