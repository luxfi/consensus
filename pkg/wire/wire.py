"""Unified sequencer stack that scales from K=1 to millions.

This module provides the wire protocol types for consensus interoperability.
AI agents and blockchain nodes communicate using the same message format.

Core Invariant:
    Everything is a Candidate with canonical ID: candidate_id = H(domain || payload)

The 4-Layer Sequencer Stack:
    1) Execution Payload - what gets executed
    2) Ordering         - who proposes, conflict resolution
    3) Data Availability - where bytes live
    4) Finality         - what proof required

Two-Phase Agreement:
    - Phase 1 (Soft): Fast, optimistic finality
    - Phase 2 (Hard): Slow, strong proof

Supported Configurations:
    - K=1: Single node (PolicyNone)
    - K=3/5: Agent mesh (PolicyQuorum)
    - K=large: Blockchain (PolicySampleConvergence + PolicyHybrid)
    - K=external: OP Stack rollup (PolicyL1Inclusion)

Usage:
    from wire import Candidate, Vote, Certificate, derive_voter_id
    from wire import SingleNodeConfig, AgentMeshConfig, BlockchainConfig

    # Create candidate
    candidate = Candidate.new(domain=b"ai-mesh", payload=b"decision text", height=1)

    # Create vote
    voter_id = derive_voter_id("claude")
    vote = Vote(candidate_id=candidate.id, voter_id=voter_id, preference=True)

    # Serialize for transmission
    data = vote.to_json()
"""

import hashlib
import json
import time
from dataclasses import dataclass, field, asdict
from typing import List, Optional
from enum import IntEnum


# =============================================================================
# CORE INVARIANT: Everything is a Candidate
# =============================================================================

def derive_voter_id(agent_id: str) -> bytes:
    """Derive a 32-byte VoterID from a string identifier."""
    return hashlib.sha256(agent_id.encode()).digest()


def derive_item_id(data: bytes) -> bytes:
    """Derive a 32-byte ItemID from arbitrary data."""
    return hashlib.sha256(data).digest()


def compute_candidate_id(domain: bytes, payload: bytes) -> bytes:
    """Compute content-addressed candidate ID: H(domain || payload)."""
    h = hashlib.sha256()
    h.update(domain)
    h.update(payload)
    return h.digest()


# =============================================================================
# POLICY IDS
# =============================================================================

class PolicyID(IntEnum):
    """Finality policy identifiers."""
    NONE = 0               # K=1 self-sequencing
    QUORUM = 1             # Threshold signature (3/5, 2/3)
    SAMPLE_CONVERGENCE = 2 # Metastable sampling (large N)
    L1_INCLUSION = 3       # External chain inclusion (OP Stack)
    QUANTUM = 4            # BLS + Ringtail post-quantum


# Signature scheme tags
SIG_NONE = 0x00
SIG_ED25519 = 0x01
SIG_BLS = 0x02
SIG_RINGTAIL = 0x03
SIG_HYBRID = 0x04


# =============================================================================
# CANDIDATE
# =============================================================================

@dataclass
class CandidateMeta:
    """Candidate metadata."""
    proposer_id: Optional[bytes] = None
    timestamp_ms: int = field(default_factory=lambda: int(time.time() * 1000))
    chain_id: Optional[bytes] = None
    extra: Optional[bytes] = None

    def to_dict(self) -> dict:
        return {
            "proposer_id": self.proposer_id.hex() if self.proposer_id else None,
            "timestamp_ms": self.timestamp_ms,
            "chain_id": self.chain_id.hex() if self.chain_id else None,
            "extra": self.extra.hex() if self.extra else None,
        }

    @classmethod
    def from_dict(cls, d: dict) -> "CandidateMeta":
        return cls(
            proposer_id=bytes.fromhex(d["proposer_id"]) if d.get("proposer_id") else None,
            timestamp_ms=d.get("timestamp_ms", int(time.time() * 1000)),
            chain_id=bytes.fromhex(d["chain_id"]) if d.get("chain_id") else None,
            extra=bytes.fromhex(d["extra"]) if d.get("extra") else None,
        )


@dataclass
class Candidate:
    """Candidate being sequenced (block, transaction, AI decision, etc.).

    Core invariant: ID = H(domain || payload)
    """
    id: bytes              # 32-byte content-addressed ID
    parent_id: bytes       # Previous candidate (optional)
    height: int            # Sequence number
    domain: bytes          # Context identifier
    payload: bytes         # Actual content
    da_ref: str = ""       # Data availability reference
    meta: CandidateMeta = field(default_factory=CandidateMeta)

    @classmethod
    def new(cls, domain: bytes, payload: bytes, height: int,
            parent_id: bytes = b'\x00' * 32) -> "Candidate":
        """Create a new candidate with computed ID."""
        candidate_id = compute_candidate_id(domain, payload)
        return cls(
            id=candidate_id,
            parent_id=parent_id,
            height=height,
            domain=domain,
            payload=payload,
        )

    def verify(self) -> bool:
        """Verify that ID matches content."""
        return self.id == compute_candidate_id(self.domain, self.payload)

    def to_dict(self) -> dict:
        return {
            "id": self.id.hex(),
            "parent_id": self.parent_id.hex(),
            "height": self.height,
            "domain": self.domain.hex(),
            "payload": self.payload.hex(),
            "da_ref": self.da_ref,
            "meta": self.meta.to_dict(),
        }

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, d: dict) -> "Candidate":
        return cls(
            id=bytes.fromhex(d["id"]),
            parent_id=bytes.fromhex(d["parent_id"]),
            height=d["height"],
            domain=bytes.fromhex(d["domain"]),
            payload=bytes.fromhex(d["payload"]),
            da_ref=d.get("da_ref", ""),
            meta=CandidateMeta.from_dict(d.get("meta", {})),
        )

    @classmethod
    def from_json(cls, data: str) -> "Candidate":
        return cls.from_dict(json.loads(data))


# =============================================================================
# VOTE
# =============================================================================

@dataclass
class Vote:
    """Attestation on a candidate."""
    candidate_id: bytes    # What's being voted on
    voter_id: bytes        # Who's voting
    round: int = 0         # Voting round
    preference: bool = True  # Accept or reject
    signature: Optional[bytes] = None  # Scheme-tagged signature
    timestamp_ms: int = field(default_factory=lambda: int(time.time() * 1000))

    def signature_scheme(self) -> int:
        """Return the signature scheme tag."""
        if not self.signature:
            return SIG_NONE
        return self.signature[0]

    def to_dict(self) -> dict:
        return {
            "candidate_id": self.candidate_id.hex(),
            "voter_id": self.voter_id.hex(),
            "round": self.round,
            "preference": self.preference,
            "signature": self.signature.hex() if self.signature else None,
            "timestamp_ms": self.timestamp_ms,
        }

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, d: dict) -> "Vote":
        return cls(
            candidate_id=bytes.fromhex(d["candidate_id"]),
            voter_id=bytes.fromhex(d["voter_id"]),
            round=d.get("round", 0),
            preference=d["preference"],
            signature=bytes.fromhex(d["signature"]) if d.get("signature") else None,
            timestamp_ms=d.get("timestamp_ms", int(time.time() * 1000)),
        )

    @classmethod
    def from_json(cls, data: str) -> "Vote":
        return cls.from_dict(json.loads(data))


# =============================================================================
# CERTIFICATE
# =============================================================================

@dataclass
class Certificate:
    """Proof of finalized agreement."""
    candidate_id: bytes    # What was finalized
    height: int            # At what height
    policy_id: PolicyID    # How finality was achieved
    proof: bytes           # Policy-specific proof
    signers: Optional[bytes] = None  # Who attested
    timestamp_ms: int = field(default_factory=lambda: int(time.time() * 1000))

    def to_dict(self) -> dict:
        return {
            "candidate_id": self.candidate_id.hex(),
            "height": self.height,
            "policy_id": int(self.policy_id),
            "proof": self.proof.hex(),
            "signers": self.signers.hex() if self.signers else None,
            "timestamp_ms": self.timestamp_ms,
        }

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, d: dict) -> "Certificate":
        return cls(
            candidate_id=bytes.fromhex(d["candidate_id"]),
            height=d["height"],
            policy_id=PolicyID(d["policy_id"]),
            proof=bytes.fromhex(d["proof"]),
            signers=bytes.fromhex(d["signers"]) if d.get("signers") else None,
            timestamp_ms=d.get("timestamp_ms", int(time.time() * 1000)),
        )

    @classmethod
    def from_json(cls, data: str) -> "Certificate":
        return cls.from_dict(json.loads(data))


# =============================================================================
# TWO-PHASE AGREEMENT
# =============================================================================

class Phase(IntEnum):
    """Agreement phase."""
    SOFT = 1  # Fast, optimistic
    HARD = 2  # Slow, strong


@dataclass
class AgreementState:
    """Tracks two-phase agreement."""
    candidate_id: bytes
    soft_finalized: bool = False
    soft_cert: Optional[Certificate] = None
    hard_finalized: bool = False
    hard_cert: Optional[Certificate] = None


# =============================================================================
# CONFIGURATION
# =============================================================================

@dataclass
class SequencerConfig:
    """Sequencer pipeline configuration."""
    domain: bytes
    k: int                 # Sample/committee size
    alpha: float           # Agreement threshold
    beta_1: float          # Soft finality threshold
    beta_2: float          # Hard finality threshold
    soft_policy: PolicyID
    hard_policy: PolicyID
    round_timeout_ms: int = 1000
    finality_timeout_ms: int = 60000

    def to_dict(self) -> dict:
        return {
            "domain": self.domain.hex(),
            "k": self.k,
            "alpha": self.alpha,
            "beta_1": self.beta_1,
            "beta_2": self.beta_2,
            "soft_policy": int(self.soft_policy),
            "hard_policy": int(self.hard_policy),
            "round_timeout_ms": self.round_timeout_ms,
            "finality_timeout_ms": self.finality_timeout_ms,
        }

    def to_json(self) -> str:
        return json.dumps(self.to_dict())


def single_node_config(domain: bytes) -> SequencerConfig:
    """Config for K=1 self-sequencing."""
    return SequencerConfig(
        domain=domain,
        k=1,
        alpha=1.0,
        beta_1=1.0,
        beta_2=1.0,
        soft_policy=PolicyID.NONE,
        hard_policy=PolicyID.NONE,
        round_timeout_ms=100,
        finality_timeout_ms=100,
    )


def agent_mesh_config(domain: bytes, k: int = 5) -> SequencerConfig:
    """Config for K=3/5 agent mesh."""
    return SequencerConfig(
        domain=domain,
        k=k,
        alpha=0.6,
        beta_1=0.5,
        beta_2=0.8,
        soft_policy=PolicyID.QUORUM,
        hard_policy=PolicyID.QUORUM,
        round_timeout_ms=5000,
        finality_timeout_ms=30000,
    )


def blockchain_config(domain: bytes) -> SequencerConfig:
    """Config for large permissionless network."""
    return SequencerConfig(
        domain=domain,
        k=20,
        alpha=0.65,
        beta_1=0.5,
        beta_2=0.8,
        soft_policy=PolicyID.SAMPLE_CONVERGENCE,
        hard_policy=PolicyID.QUANTUM,
        round_timeout_ms=1000,
        finality_timeout_ms=60000,
    )


def rollup_config(domain: bytes) -> SequencerConfig:
    """Config for OP Stack style rollup."""
    return SequencerConfig(
        domain=domain,
        k=1,
        alpha=1.0,
        beta_1=1.0,
        beta_2=1.0,
        soft_policy=PolicyID.NONE,
        hard_policy=PolicyID.L1_INCLUSION,
        round_timeout_ms=2000,
        finality_timeout_ms=600000,  # 10 minutes
    )


# =============================================================================
# LEGACY COMPATIBILITY
# =============================================================================

@dataclass
class ConsensusParams:
    """Legacy consensus parameters (for backward compatibility)."""
    k: int = 3
    alpha: float = 0.6
    beta_1: float = 0.5
    beta_2: float = 0.8
    rounds: int = 3

    def to_dict(self) -> dict:
        return asdict(self)

    def to_json(self) -> str:
        return json.dumps(self.to_dict())


def default_params() -> ConsensusParams:
    """Default consensus parameters."""
    return ConsensusParams()


def blockchain_params() -> ConsensusParams:
    """Blockchain-tuned parameters."""
    return ConsensusParams(k=20, alpha=0.65, beta_1=0.5, beta_2=0.8, rounds=10)


def ai_agent_params() -> ConsensusParams:
    """AI agent-tuned parameters."""
    return ConsensusParams(k=3, alpha=0.6, beta_1=0.5, beta_2=0.8, rounds=3)


# Legacy type aliases
ItemID = bytes
VoterID = bytes
Result = Certificate  # Legacy alias


# =============================================================================
# VALIDATOR SET (for Membership interface)
# =============================================================================

@dataclass
class Validator:
    """Participant in consensus."""
    id: bytes              # Voter identifier
    weight: int = 1        # Voting power
    public_key: Optional[bytes] = None
    transport_addr: str = ""

    def to_dict(self) -> dict:
        return {
            "id": self.id.hex(),
            "weight": self.weight,
            "public_key": self.public_key.hex() if self.public_key else None,
            "transport_addr": self.transport_addr,
        }

    @classmethod
    def from_dict(cls, d: dict) -> "Validator":
        return cls(
            id=bytes.fromhex(d["id"]),
            weight=d.get("weight", 1),
            public_key=bytes.fromhex(d["public_key"]) if d.get("public_key") else None,
            transport_addr=d.get("transport_addr", ""),
        )


@dataclass
class ValidatorSet:
    """Set of validators for an epoch."""
    epoch: int
    validators: List[Validator]
    total_weight: int = 0

    def __post_init__(self):
        if self.total_weight == 0:
            self.total_weight = sum(v.weight for v in self.validators)

    def to_dict(self) -> dict:
        return {
            "epoch": self.epoch,
            "validators": [v.to_dict() for v in self.validators],
            "total_weight": self.total_weight,
        }

    def to_json(self) -> str:
        return json.dumps(self.to_dict())

    @classmethod
    def from_dict(cls, d: dict) -> "ValidatorSet":
        return cls(
            epoch=d["epoch"],
            validators=[Validator.from_dict(v) for v in d.get("validators", [])],
            total_weight=d.get("total_weight", 0),
        )

    @classmethod
    def from_json(cls, data: str) -> "ValidatorSet":
        return cls.from_dict(json.loads(data))
