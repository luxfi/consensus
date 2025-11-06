#!/usr/bin/env python3
"""
Python client for Lux cross-chain bridge.

This demonstrates how to interact with the Go bridge service from Python.
"""

import asyncio
import grpc
from typing import Optional
import time

# In production, these would be generated from proto files
# For this example, we'll use a simple REST-like approach
import requests
import json


class BridgeClient:
    """Python client for Lux bridge service."""
    
    def __init__(self, endpoint: str = "http://localhost:8080"):
        self.endpoint = endpoint
        self.session = requests.Session()
        
    def transfer(self, asset: str, amount: str, source_chain: str, dest_chain: str) -> str:
        """
        Initiate a cross-chain transfer.
        
        Args:
            asset: Asset symbol (e.g., "ETH", "LUX")
            amount: Amount in smallest units (wei)
            source_chain: Source blockchain
            dest_chain: Destination blockchain
            
        Returns:
            Transfer ID
        """
        data = {
            "asset": asset,
            "amount": amount,
            "source_chain": source_chain,
            "dest_chain": dest_chain,
        }
        
        response = self.session.post(f"{self.endpoint}/transfer", json=data)
        response.raise_for_status()
        
        result = response.json()
        return result["transfer_id"]
    
    def get_status(self, transfer_id: str) -> dict:
        """Get transfer status."""
        response = self.session.get(f"{self.endpoint}/transfer/{transfer_id}")
        response.raise_for_status()
        
        return response.json()
    
    def wait_for_completion(self, transfer_id: str, timeout: int = 60) -> dict:
        """Wait for transfer to complete with timeout."""
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            status = self.get_status(transfer_id)
            
            print(f"[{status['status']}] {status['confirmations']}/{status['required_confirmations']} confirmations")
            
            if status["status"] == "COMPLETED":
                return status
            
            if status["status"] == "FAILED":
                raise Exception(f"Transfer failed: {status.get('error')}")
            
            time.sleep(1)
        
        raise TimeoutError(f"Transfer did not complete within {timeout}s")
    
    def get_exchange_rate(self, from_chain: str, to_chain: str) -> str:
        """Get exchange rate between chains."""
        response = self.session.get(
            f"{self.endpoint}/rate",
            params={"from": from_chain, "to": to_chain}
        )
        response.raise_for_status()
        
        result = response.json()
        return result["rate"]


def format_amount(amount: str, decimals: int = 18) -> str:
    """Format amount from wei to human-readable."""
    value = int(amount) / (10 ** decimals)
    return f"{value:.4f}"


def main():
    """Run example transfers."""
    print("=== Python Bridge Client ===\n")
    
    # Create client
    client = BridgeClient("http://localhost:8080")
    
    print("Initiating transfer:")
    print("  Amount: 1.5 ETH")
    print("  From:   ethereum")
    print("  To:     lux")
    print()
    
    try:
        # Initiate transfer (1.5 ETH in wei)
        amount = "1500000000000000000"
        transfer_id = client.transfer(
            asset="ETH",
            amount=amount,
            source_chain="ethereum",
            dest_chain="lux",
        )
        
        print(f"✓ Transfer initiated: {transfer_id}\n")
        print("✓ Monitoring status...\n")
        
        # Wait for completion
        result = client.wait_for_completion(transfer_id, timeout=30)
        
        print(f"\n✓ Transfer completed!\n")
        print("Transfer Details:")
        print(f"  ID:        {result['id']}")
        print(f"  Amount:    {format_amount(result['amount'])} {result['asset']}")
        print(f"  Status:    {result['status']}")
        print(f"  From:      {result['source_chain']}")
        print(f"  To:        {result['dest_chain']}")
        
        if "source_tx_hash" in result:
            print(f"  Src Hash:  {result['source_tx_hash']}")
        if "dest_tx_hash" in result:
            print(f"  Dest Hash: {result['dest_tx_hash']}")
        
    except requests.exceptions.ConnectionError:
        print("✗ Error: Could not connect to bridge service")
        print("  Make sure the bridge server is running:")
        print("  cd server && go run main.go")
    except Exception as e:
        print(f"✗ Error: {e}")


if __name__ == "__main__":
    main()
