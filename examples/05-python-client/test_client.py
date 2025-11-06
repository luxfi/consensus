"""
Tests for Python bridge client.

Run with: pytest test_client.py -v
"""

import pytest
from unittest.mock import Mock, patch
from client import BridgeClient, format_amount


@pytest.fixture
def mock_client():
    """Create a mock bridge client."""
    with patch('client.requests.Session') as mock_session:
        client = BridgeClient("http://localhost:8080")
        client.session = mock_session.return_value
        yield client, mock_session.return_value


def test_transfer_initiation(mock_client):
    """Test initiating a transfer."""
    client, mock_session = mock_client
    
    # Mock response
    mock_response = Mock()
    mock_response.json.return_value = {"transfer_id": "tx_123"}
    mock_response.raise_for_status = Mock()
    mock_session.post.return_value = mock_response
    
    # Call transfer
    transfer_id = client.transfer(
        asset="ETH",
        amount="1000000000000000000",
        source_chain="ethereum",
        dest_chain="lux",
    )
    
    assert transfer_id == "tx_123"
    
    # Verify API was called correctly
    mock_session.post.assert_called_once()
    call_args = mock_session.post.call_args
    assert call_args[0][0] == "http://localhost:8080/transfer"
    assert call_args[1]["json"]["asset"] == "ETH"
    assert call_args[1]["json"]["amount"] == "1000000000000000000"


def test_get_status(mock_client):
    """Test getting transfer status."""
    client, mock_session = mock_client
    
    # Mock response
    mock_response = Mock()
    mock_response.json.return_value = {
        "id": "tx_123",
        "status": "COMPLETED",
        "confirmations": 6,
        "required_confirmations": 6,
    }
    mock_response.raise_for_status = Mock()
    mock_session.get.return_value = mock_response
    
    # Call get_status
    status = client.get_status("tx_123")
    
    assert status["id"] == "tx_123"
    assert status["status"] == "COMPLETED"
    assert status["confirmations"] == 6


def test_get_exchange_rate(mock_client):
    """Test getting exchange rate."""
    client, mock_session = mock_client
    
    # Mock response
    mock_response = Mock()
    mock_response.json.return_value = {"rate": "1500"}
    mock_response.raise_for_status = Mock()
    mock_session.get.return_value = mock_response
    
    # Call get_exchange_rate
    rate = client.get_exchange_rate("ethereum", "lux")
    
    assert rate == "1500"


def test_format_amount():
    """Test amount formatting."""
    # Test 1 ETH
    assert format_amount("1000000000000000000") == "1.0000"
    
    # Test 1.5 ETH
    assert format_amount("1500000000000000000") == "1.5000"
    
    # Test 0.01 ETH
    assert format_amount("10000000000000000") == "0.0100"
    
    # Test 100 LUX (assuming 18 decimals)
    assert format_amount("100000000000000000000") == "100.0000"


def test_wait_for_completion_success(mock_client):
    """Test waiting for transfer completion - success case."""
    client, mock_session = mock_client
    
    # Mock responses - pending then completed
    mock_responses = [
        {
            "id": "tx_123",
            "status": "PENDING",
            "confirmations": 2,
            "required_confirmations": 6,
        },
        {
            "id": "tx_123",
            "status": "COMPLETED",
            "confirmations": 6,
            "required_confirmations": 6,
            "source_tx_hash": "0xabc...",
            "dest_tx_hash": "0xdef...",
        },
    ]
    
    mock_response = Mock()
    mock_response.json.side_effect = mock_responses
    mock_response.raise_for_status = Mock()
    mock_session.get.return_value = mock_response
    
    # Call wait_for_completion
    result = client.wait_for_completion("tx_123", timeout=5)
    
    assert result["status"] == "COMPLETED"
    assert result["confirmations"] == 6


def test_wait_for_completion_failed(mock_client):
    """Test waiting for transfer completion - failure case."""
    client, mock_session = mock_client
    
    # Mock failed response
    mock_response = Mock()
    mock_response.json.return_value = {
        "id": "tx_123",
        "status": "FAILED",
        "error": "Insufficient liquidity",
    }
    mock_response.raise_for_status = Mock()
    mock_session.get.return_value = mock_response
    
    # Call wait_for_completion - should raise exception
    with pytest.raises(Exception, match="Transfer failed"):
        client.wait_for_completion("tx_123", timeout=5)


@pytest.mark.parametrize("amount,expected", [
    ("1000000000000000000", "1.0000"),
    ("500000000000000000", "0.5000"),
    ("2500000000000000000", "2.5000"),
])
def test_format_amount_parametrized(amount, expected):
    """Test amount formatting with multiple inputs."""
    assert format_amount(amount) == expected
