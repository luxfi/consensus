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
    from lux_consensus.types import Candidate, Vote, Certificate
    from lux_consensus.types import single_node_config, agent_mesh_config

    # Create candidate
    candidate = Candidate.new(domain=b"ai-mesh", payload=b"decision text", height=1)

    # Create vote
    voter_id = voter_id_from_agent("claude")
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

# =============================================================================
# NODE ID DOMAIN (matches node repo)
# =============================================================================
NODE_ID_DOMAIN = "LuxNodeID/v1"


def derive_voter_id(domain: str, data: bytes) -> bytes:
    """Derive a 32-byte VoterID: H(domain || data).

    This is the single canonical derivation function.

    Args:
        domain: Context identifier (e.g., "LuxNodeID/v1" for validators, "agent" for AI)
        data: Raw bytes to hash (e.g., public key, agent name)

    Returns:
        32-byte VoterID

    Examples:
        # For blockchain validators (matches NodeID)
        voter_id = derive_voter_id(NODE_ID_DOMAIN, mldsa_public_key)

        # For AI agents
        voter_id = derive_voter_id("agent", b"claude")
    """
    h = hashlib.sha256()
    h.update(domain.encode())
    h.update(data)
    return h.digest()


def voter_id_from_public_key(public_key: bytes) -> bytes:
    """Derive VoterID from public key using NODE_ID_DOMAIN.

    This ensures VoterID == NodeID for the same public key.
    """
    return derive_voter_id(NODE_ID_DOMAIN, public_key)


def voter_id_from_agent(agent_name: str) -> bytes:
    """Derive VoterID for an AI agent."""
    return derive_voter_id("agent", agent_name.encode())


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
SIG_QUASAR = 0x04  # BLS + Ringtail (Quasar protocol)


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


# =============================================================================
# SEQUENCER TYPE: Abstract any sequencer (native, external, recursive)
# =============================================================================

class SequencerType(IntEnum):
    """Sequencer type identifier."""
    NATIVE = 0          # Lux native consensus
    EXTERNAL = 1        # External sequencer (OP Stack, Arbitrum, etc.)
    RECURSIVE = 2       # Child network with parent finality


@dataclass
class SequencerIdentity:
    """Identifies a specific sequencer within a network topology.

    A sequencer can be:
    - Native: Lux consensus engine
    - External: OP Stack, Arbitrum, zkSync, etc.
    - Recursive: Child chain whose finality derives from parent
    """
    sequencer_type: SequencerType
    chain_id: int                      # Unique chain identifier
    domain: bytes                      # Consensus domain
    parent_chain_id: Optional[int] = None  # For recursive networks
    external_rpc: Optional[str] = None     # For external sequencers
    depth: int = 0                         # Recursion depth (0 = root)

    def to_dict(self) -> dict:
        return {
            "sequencer_type": int(self.sequencer_type),
            "chain_id": self.chain_id,
            "domain": self.domain.hex(),
            "parent_chain_id": self.parent_chain_id,
            "external_rpc": self.external_rpc,
            "depth": self.depth,
        }

    @classmethod
    def from_dict(cls, d: dict) -> "SequencerIdentity":
        return cls(
            sequencer_type=SequencerType(d["sequencer_type"]),
            chain_id=d["chain_id"],
            domain=bytes.fromhex(d["domain"]),
            parent_chain_id=d.get("parent_chain_id"),
            external_rpc=d.get("external_rpc"),
            depth=d.get("depth", 0),
        )


def native_sequencer(chain_id: int, domain: bytes) -> SequencerIdentity:
    """Create a native Lux sequencer identity."""
    return SequencerIdentity(
        sequencer_type=SequencerType.NATIVE,
        chain_id=chain_id,
        domain=domain,
    )


def external_sequencer(chain_id: int, domain: bytes, rpc: str) -> SequencerIdentity:
    """Create an external sequencer identity (OP Stack, Arbitrum, etc.)."""
    return SequencerIdentity(
        sequencer_type=SequencerType.EXTERNAL,
        chain_id=chain_id,
        domain=domain,
        external_rpc=rpc,
    )


def recursive_sequencer(
    chain_id: int,
    domain: bytes,
    parent_chain_id: int,
    depth: int = 1
) -> SequencerIdentity:
    """Create a recursive child sequencer identity."""
    return SequencerIdentity(
        sequencer_type=SequencerType.RECURSIVE,
        chain_id=chain_id,
        domain=domain,
        parent_chain_id=parent_chain_id,
        depth=depth,
    )


# =============================================================================
# RECURSIVE NETWORK TOPOLOGY: Infinite fractal networks
# =============================================================================

@dataclass
class NetworkNode:
    """Node in the recursive network topology.

    Each node represents a chain that can have:
    - Zero or more child chains (recursive)
    - At most one parent chain (except root)
    - Its own sequencer configuration
    """
    identity: SequencerIdentity
    config: SequencerConfig
    children: List["NetworkNode"] = field(default_factory=list)

    def add_child(self, child: "NetworkNode") -> None:
        """Add a child chain to this node."""
        # Update child's parent reference
        child.identity.parent_chain_id = self.identity.chain_id
        child.identity.depth = self.identity.depth + 1
        self.children.append(child)

    def traverse(self) -> List["NetworkNode"]:
        """Traverse all nodes in the network (depth-first)."""
        result = [self]
        for child in self.children:
            result.extend(child.traverse())
        return result

    def find_by_chain_id(self, chain_id: int) -> Optional["NetworkNode"]:
        """Find a node by chain ID."""
        if self.identity.chain_id == chain_id:
            return self
        for child in self.children:
            found = child.find_by_chain_id(chain_id)
            if found:
                return found
        return None

    def to_dict(self) -> dict:
        return {
            "identity": self.identity.to_dict(),
            "config": self.config.to_dict(),
            "children": [c.to_dict() for c in self.children],
        }

    @classmethod
    def from_dict(cls, d: dict) -> "NetworkNode":
        node = cls(
            identity=SequencerIdentity.from_dict(d["identity"]),
            config=SequencerConfig(
                domain=bytes.fromhex(d["config"]["domain"]),
                k=d["config"]["k"],
                alpha=d["config"]["alpha"],
                beta_1=d["config"]["beta_1"],
                beta_2=d["config"]["beta_2"],
                soft_policy=PolicyID(d["config"]["soft_policy"]),
                hard_policy=PolicyID(d["config"]["hard_policy"]),
                round_timeout_ms=d["config"].get("round_timeout_ms", 1000),
                finality_timeout_ms=d["config"].get("finality_timeout_ms", 60000),
            ),
        )
        for child_dict in d.get("children", []):
            node.children.append(cls.from_dict(child_dict))
        return node


@dataclass
class RecursiveNetwork:
    """Fractal recursive network topology.

    Supports infinite depth of child chains, each with their own:
    - Sequencer configuration
    - Finality policy
    - Validator set

    Finality flows UP the tree:
    - Child chain finalizes locally
    - Parent chain includes child certificate
    - Root chain provides global finality

    Example topology:
        Lux Mainnet (root)
        ├── AI Mesh Chain (k=5, quorum)
        │   ├── Agent Subnetwork A (k=3)
        │   └── Agent Subnetwork B (k=3)
        ├── Gaming Chain (k=20, metastable)
        │   └── Game Instance (k=1, single)
        └── DeFi Chain (k=100, PQ)
            ├── AMM Subnet
            └── Bridge Subnet
    """
    root: NetworkNode

    def get_all_chains(self) -> List[NetworkNode]:
        """Get all chains in the network."""
        return self.root.traverse()

    def get_chain(self, chain_id: int) -> Optional[NetworkNode]:
        """Get a chain by ID."""
        return self.root.find_by_chain_id(chain_id)

    def get_finality_path(self, chain_id: int) -> List[NetworkNode]:
        """Get the path from a chain to the root (finality path)."""
        path = []
        node = self.get_chain(chain_id)
        while node:
            path.append(node)
            if node.identity.parent_chain_id is None:
                break
            node = self.get_chain(node.identity.parent_chain_id)
        return path

    def add_chain(
        self,
        parent_chain_id: int,
        child_identity: SequencerIdentity,
        child_config: SequencerConfig,
    ) -> bool:
        """Add a child chain under a parent."""
        parent = self.get_chain(parent_chain_id)
        if not parent:
            return False
        child_node = NetworkNode(identity=child_identity, config=child_config)
        parent.add_child(child_node)
        return True

    def to_dict(self) -> dict:
        return {"root": self.root.to_dict()}

    @classmethod
    def from_dict(cls, d: dict) -> "RecursiveNetwork":
        return cls(root=NetworkNode.from_dict(d["root"]))


# Factory functions for common topologies

def single_chain_network(
    chain_id: int,
    domain: bytes,
    config: SequencerConfig,
) -> RecursiveNetwork:
    """Create a simple single-chain network."""
    return RecursiveNetwork(
        root=NetworkNode(
            identity=native_sequencer(chain_id, domain),
            config=config,
        )
    )


def ai_mesh_network(
    root_chain_id: int,
    domain: bytes,
    agent_count: int = 5,
) -> RecursiveNetwork:
    """Create an AI agent mesh network."""
    root = NetworkNode(
        identity=native_sequencer(root_chain_id, domain),
        config=agent_mesh_config(domain, k=agent_count),
    )
    return RecursiveNetwork(root=root)


def recursive_rollup_network(
    l1_chain_id: int,
    l1_domain: bytes,
    l2_configs: List[tuple],  # List of (chain_id, domain, config)
) -> RecursiveNetwork:
    """Create a recursive rollup network with multiple L2s.

    Args:
        l1_chain_id: L1 chain ID
        l1_domain: L1 domain
        l2_configs: List of (chain_id, domain, config) for each L2
    """
    root = NetworkNode(
        identity=native_sequencer(l1_chain_id, l1_domain),
        config=blockchain_config(l1_domain),
    )

    for l2_chain_id, l2_domain, l2_config in l2_configs:
        l2_node = NetworkNode(
            identity=recursive_sequencer(l2_chain_id, l2_domain, l1_chain_id),
            config=l2_config,
        )
        root.add_child(l2_node)

    return RecursiveNetwork(root=root)


# =============================================================================
# BRIDGE TO HANZO-CONSENSUS: Convert AI consensus to wire protocol
# =============================================================================

def hanzo_result_to_vote(
    result_id: str,
    result_output: str,
    candidate_id: bytes,
    preference: bool,
) -> Vote:
    """Convert a Hanzo consensus Result to a wire Vote.

    Args:
        result_id: Hanzo participant ID (string)
        result_output: Output text (for synthesis)
        candidate_id: The candidate being voted on
        preference: Vote direction

    Returns:
        Wire protocol Vote
    """
    voter_id = voter_id_from_agent(result_id)
    return Vote(
        candidate_id=candidate_id,
        voter_id=voter_id,
        preference=preference,
    )


def hanzo_state_to_certificate(
    prompt: str,
    winner: str,
    synthesis: str,
    participants: List[str],
    confidence: float,
) -> Certificate:
    """Convert a Hanzo consensus State to a wire Certificate.

    Args:
        prompt: Original query
        winner: Winning participant ID
        synthesis: Final synthesis text
        participants: All participant IDs
        confidence: Final confidence score

    Returns:
        Wire protocol Certificate
    """
    # Candidate ID is hash of prompt (what we agreed on)
    candidate_id = compute_candidate_id(b"ai-mesh", prompt.encode())

    # Encode synthesis in proof
    proof = json.dumps({
        "winner": winner,
        "synthesis": synthesis,
        "confidence": confidence,
    }).encode()

    # Signers bitmap (simplified)
    signers = bytes([1] * ((len(participants) + 7) // 8))

    return Certificate(
        candidate_id=candidate_id,
        height=0,  # AI consensus doesn't have block height
        policy_id=PolicyID.QUORUM,
        proof=proof,
        signers=signers,
    )


def create_ai_candidate(prompt: str, domain: bytes = b"ai-mesh") -> Candidate:
    """Create a Candidate for AI consensus.

    Args:
        prompt: The query to reach consensus on
        domain: Domain identifier (default: "ai-mesh")

    Returns:
        Wire protocol Candidate
    """
    return Candidate.new(
        domain=domain,
        payload=prompt.encode(),
        height=0,
    )
